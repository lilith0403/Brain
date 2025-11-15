import { Injectable, OnModuleInit } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { IngestDto } from './dtos/ingest.dto';
import { AskDto } from './dtos/ask.dto';
import { GoogleGenerativeAIEmbeddings } from '@langchain/google-genai';
import { Chroma } from '@langchain/community/vectorstores/chroma';
import { ChromaClient } from 'chromadb-client';
import { RecursiveCharacterTextSplitter, MarkdownTextSplitter, SupportedTextSplitterLanguage } from '@langchain/textsplitters';
import * as path from 'path';
import { ChatPromptTemplate, PromptTemplate } from '@langchain/core/prompts';
import { ChatGoogleGenerativeAI } from '@langchain/google-genai';
import { StringOutputParser } from '@langchain/core/output_parsers';
import { RunnableSequence } from '@langchain/core/runnables';
import { Document } from '@langchain/core/documents';
import { CohereRerank } from '@langchain/cohere';
import { AgentExecutor, createReactAgent } from '@langchain/classic/agents';
import { z } from 'zod';
import { DynamicTool } from '@langchain/core/tools';
import { pull } from 'langchain/hub'

function formatDocumentsAsString(documents: Document[]): string {
  return documents.map((doc) => doc.pageContent).join('\n\n');
}


@Injectable()
export class AiService implements OnModuleInit {
  
  private embeddings: GoogleGenerativeAIEmbeddings;
  private vectorStore: Chroma;
  private model: ChatGoogleGenerativeAI;
  private agentExecutor: AgentExecutor;

  constructor(private configService: ConfigService) {}

  async onModuleInit() {

    const chromaHost = this.configService.get<string>('CHROMA_HOST') || 'localhost';
    const chromaPort = this.configService.get<string>('CHROMA_PORT') || '8000';
    const chromaUrl = `http://${chromaHost}:${chromaPort}`;
    
    const googleApiKey = this.configService.get<string>('GOOGLE_API_KEY');
    if (!googleApiKey) {
      throw new Error('GOOGLE_API_KEY não encontrada no .env');
    }

    this.model = new ChatGoogleGenerativeAI({
        apiKey: googleApiKey,
        model: 'gemini-2.5-flash',
        temperature: 0.3,
      });

    this.embeddings = new GoogleGenerativeAIEmbeddings({
      apiKey: googleApiKey,
      modelName: 'text-embedding-004',
    });



    console.log(`AiService tentando se conectar ao ChromaDB em: ${chromaUrl}`);
    const tempClient = new ChromaClient({ path: chromaUrl });
    try {
      await tempClient.getCollection({ name: 'brain-collection' });
    } catch (error) {
      console.log('Coleção não encontrada, criando...');
      await tempClient.createCollection({
        name: 'brain-collection',
      });
    }

    this.vectorStore = new Chroma(this.embeddings, {
      url: chromaUrl,
      collectionName: 'brain-collection',
    });

    const tools = [this.createSearchTool()];

    const prompt = await pull<PromptTemplate>('hwchase17/react');

    const agent = await createReactAgent({
      llm: this.model,
      tools: tools,
      prompt,
    });

    this.agentExecutor = new AgentExecutor({
      agent,
      tools: tools,
      verbose: true,
    });

    console.log('AiService (com Agente ReAct) inicializado com sucesso!');
  }

  private createSearchTool(): DynamicTool {
    const retriever = this.vectorStore.asRetriever(300);
    const reranker = new CohereRerank({
      apiKey: this.configService.get<string>('COHERE_API_KEY'),
      model: 'rerank-multilingual-v3.0',
      topN: 3,
    });

    return new DynamicTool({
      name: 'search_files',
      description: `Busca nos arquivos pessoais do usuário (código, configs, notas) para encontrar contexto. 
                    Use isso para responder perguntas sobre as configurações do usuário, o que é um
                    certo arquivo, ou para encontrar definições de funções/classes.`,
      func: async (query: string) => {
        try {
          console.log(`[Tool: search_files] Buscando por: "${query}"`);
          const initialDocs = await retriever.invoke(query);
          if (initialDocs.length === 0) {
            return 'Nenhum documento encontrado.';
          }

          const rerankedDocs = await reranker.compressDocuments(
            initialDocs,
            query,
          );
          if (rerankedDocs.length === 0) {
            return 'Nenhum documento relevante encontrado após re-ranking.';
          }

          return formatDocumentsAsString(rerankedDocs);
        } catch (error) {
          console.error('[Tool: search_files] Erro:', error);
          return 'Erro ao tentar buscar nos arquivos.';
        }
      },
    });
  }

  /**
   * 
   * Esta função mapeia extensões de arquivo para linguagens suportadas pelo LangChain.
   * O mapeamento é crucial porque diferentes tipos de arquivo têm estruturas diferentes:
   * 
   * - Código (JS, TS, Python, etc.): Precisa preservar funções, classes, blocos
   * - Markdown: Precisa preservar estrutura de títulos, listas, blocos de código
   * - Configuração (.conf, .toml, .yaml): Estrutura hierárquica
   * - Texto puro: Sem estrutura especial, pode quebrar por parágrafos
   * 
   * @param filePath - Caminho completo do arquivo
   * @returns Linguagem suportada ou null se não reconhecida
   */
  private detectLanguageFromFile(filePath: string): SupportedTextSplitterLanguage | null {
    const ext = path.extname(filePath).toLowerCase();
    
    const extensionMap: Record<string, SupportedTextSplitterLanguage> = {
      '.js': 'js',
      '.jsx': 'js',
      '.ts': 'js',
      '.tsx': 'js',
      
      '.py': 'python',
      '.pyw': 'python',
      
      '.java': 'java',
      
      '.cpp': 'cpp',
      '.cc': 'cpp',
      '.cxx': 'cpp',
      '.c': 'cpp',
      '.h': 'cpp',
      '.hpp': 'cpp',
      '.hxx': 'cpp',
      
      '.go': 'go',
      
      '.rs': 'rust',
      
      '.rb': 'ruby',
      '.rbw': 'ruby',
      
      '.php': 'php',
      '.phtml': 'php',
      
      '.swift': 'swift',
      
      '.scala': 'scala',
      
      '.proto': 'proto',
      
      '.rst': 'rst',
      
      '.sol': 'sol',
      
      '.md': 'markdown',
      '.markdown': 'markdown',
      
      '.html': 'html',
      '.htm': 'html',
      '.xhtml': 'html',
      
      '.latex': 'latex',
      '.tex': 'latex',
    };

    return extensionMap[ext] || null;
  }

  /**
   * 
   * Diferentes tipos de arquivo requerem diferentes estratégias:
   * 
   * 1. MARKDOWN: Usa MarkdownTextSplitter que preserva:
   *    - Estrutura de títulos (# ## ###)
   *    - Blocos de código (```)
   *    - Listas e parágrafos
   *    - Links e imagens
   * 
   * 2. CÓDIGO: Usa RecursiveCharacterTextSplitter.fromLanguage() que:
   *    - Respeita separadores específicos da linguagem (; {} () [])
   *    - Preserva funções e classes completas
   *    - Mantém contexto entre blocos relacionados
   *    - ChunkSize maior (1500) porque código tem mais contexto
   *    - ChunkOverlap maior (300) para manter contexto entre funções
   * 
   * 3. TEXTO/CONFIG: Usa RecursiveCharacterTextSplitter genérico que:
   *    - Quebra por parágrafos (\n\n)
   *    - Depois por linhas (\n)
   *    - Depois por palavras
   *    - ChunkSize menor (1000) para textos mais simples
   *    - ChunkOverlap menor (100) suficiente para contexto
   * 
   * @param filePath - Caminho do arquivo para determinar estratégia
   * @returns Text splitter configurado apropriadamente
   */
  private getTextSplitterForFile(filePath: string): RecursiveCharacterTextSplitter | MarkdownTextSplitter {
    const language = this.detectLanguageFromFile(filePath);
    const ext = path.extname(filePath).toLowerCase();
    
    if (ext === '.md' || ext === '.markdown') {
      console.log(`[Chunking] Usando MarkdownTextSplitter para: ${filePath}`);
      return new MarkdownTextSplitter({
        chunkSize: 2000,
        chunkOverlap: 200,
      });
    }
    
    if (language) {
      console.log(`[Chunking] Usando CodeTextSplitter (${language}) para: ${filePath}`);
      return RecursiveCharacterTextSplitter.fromLanguage(language, {
        chunkSize: 1500,
        chunkOverlap: 300,
      });
    }

    console.log(`[Chunking] Usando RecursiveCharacterTextSplitter (genérico) para: ${filePath}`);
    return new RecursiveCharacterTextSplitter({
      chunkSize: 1000,
      chunkOverlap: 100,
      separators: [
        '\n\n',
        '\n',
        '. ',
        ' ',
        '',
      ],
    });
  }

  /**
   * 
   * O processo de chunking inteligente funciona assim:
   * 
   * 1. Detecta o tipo de arquivo pela extensão
   * 2. Seleciona o splitter apropriado
   * 3. Aplica o splitter no conteúdo
   * 4. Cada chunk mantém metadados (source, lastModified)
   * 5. Chunks são armazenados no vector store
   * 
   * Por que isso é importante?
   * - Chunks bem formados = melhor embedding = melhor busca semântica
   * - Preservar estrutura = contexto mais rico = respostas mais precisas
   * - Overlap adequado = não perder contexto entre chunks relacionados
   */
  async ingest(ingestDto: IngestDto) {
    const { content, filePath, lastModified } = ingestDto;
    const newModTime = new Date(lastModified);

    try {
      const existing = await this.vectorStore.similaritySearch(' ', 1, {
        source: filePath,
      });
      if (existing && existing.length > 0) {
        const oldModTime = new Date(
          existing[0].metadata.lastModified as string,
        );
        if (oldModTime >= newModTime) {
          console.log(`IGNORADO (atualizado): ${filePath}`);
          return {
            status: 'ok',
            message: `Arquivo ${filePath} já está atualizado.`,
          };
        }
      }

      console.log(`Iniciando "Delete-then-Add" para: ${filePath}`);

      await this.vectorStore.delete({ filter: { source: filePath } });

      const splitter = this.getTextSplitterForFile(filePath);

      const chunks = await splitter.createDocuments(
        [content],
        [{ source: filePath, lastModified: newModTime.toISOString() }],
      );

      await this.vectorStore.addDocuments(chunks);

      console.log(
        `Arquivo ${filePath} RE-INDEXADO com sucesso. ${chunks.length} chunks criados usando estratégia otimizada.`,
      );
      return {
        status: 'ok',
        message: `Arquivo ${filePath} re-indexado. ${chunks.length} chunks criados.`,
      };
    } catch (error) {
      console.error(`Erro durante a ingestão "${filePath}":`, error);
      throw error;
    }
  }
  async ask(askDto: AskDto) {
    const { query } = askDto;
    console.log(`[Agente] Recebida a query: ${askDto.query}`);
    try {
      const result = await this.agentExecutor.invoke({ input: query });
      console.log(`[Agente] Resposta: ${result.output}`);
      return {
        status: 'ok',
        answer: result.output,
      };
    } catch (error: any) {
      const errorMessage = error?.message || '';
      let extractedAnswer: string | null = null;
      
      if (errorMessage.includes('Parsing LLM output produced both a final answer')) {
        const finalAnswerMatch = errorMessage.match(/Final Answer:\s*([\s\S]+?)(?:\n\nTroubleshooting URL:|$)/i);
        
        if (finalAnswerMatch && finalAnswerMatch[1]) {
          let answer = finalAnswerMatch[1].trim();
          answer = answer.replace(/\n{3,}/g, '\n\n').trim();
          extractedAnswer = answer;
          console.log(`[Agente] Extraída resposta final do erro (LLM retornou ação + resposta)`);
        }
      }
      
      if (!extractedAnswer && errorMessage.includes('Could not parse LLM output:')) {
        const parseErrorMatch = errorMessage.match(/Could not parse LLM output:\s*([\s\S]+?)(?:\n\nTroubleshooting URL:|$)/i);
        
        if (parseErrorMatch && parseErrorMatch[1]) {
          let answer = parseErrorMatch[1].trim();
          
          answer = answer.replace(/\n\nTroubleshooting URL:.*$/i, '').trim();
          
          answer = answer.replace(/^Error: Could not parse LLM output:\s*/i, '').trim();
          
          if (answer && 
              !answer.includes('Action:') && 
              !answer.includes('Thought:') &&
              answer.length > 50 &&
              /(?:para|você|seu|arquivo|editar|mudar|cor|texto|waybar|css|style|localize|encontre|adicione|modifique|salve|reinicie|experimente|pode|precisa|deve|geralmente|normalmente|exemplo|passos|resumo|abordagem)/i.test(answer)) {
            extractedAnswer = answer;
            console.log(`[Agente] Extraída resposta direta do erro (LLM não seguiu formato ReAct)`);
          }
        }
      }
      
      if (extractedAnswer) {
        return {
          status: 'ok',
          answer: extractedAnswer,
        };
      }
      
      console.error('Erro durante o RAG do Agente (não foi possível extrair resposta válida):', error);
      return { status: 'error', answer: 'Ocorreu um erro ao processar sua pergunta.' };
    }
  }
}
