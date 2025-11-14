import { Injectable, OnModuleInit } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { IngestDto } from './dtos/ingest.dto';
import { AskDto } from './dtos/ask.dto';
import { GoogleGenerativeAIEmbeddings } from '@langchain/google-genai';
import { Chroma } from '@langchain/community/vectorstores/chroma';
import { ChromaClient } from 'chromadb-client';
import { RecursiveCharacterTextSplitter } from '@langchain/textsplitters';
import { RunnableSequence } from '@langchain/core/runnables';
import { ChatPromptTemplate } from '@langchain/core/prompts';
import { ChatGoogleGenerativeAI } from '@langchain/google-genai';
import { StringOutputParser } from '@langchain/core/output_parsers';
import { Document } from '@langchain/core/documents';

function formatDocumentsAsString(documents: Document[]): string {
  return documents.map((doc) => doc.pageContent).join('\n\n');
}

@Injectable()
export class AiService implements OnModuleInit {
  
  private embeddings: GoogleGenerativeAIEmbeddings;
  private vectorStore: Chroma;
  private textSplitter: RecursiveCharacterTextSplitter;
  private model: ChatGoogleGenerativeAI;

  constructor(private configService: ConfigService) {}

  async onModuleInit() {
    
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

    this.textSplitter = new RecursiveCharacterTextSplitter({
      chunkSize: 1000,
      chunkOverlap: 100,
    });


  const chromaClient = new ChromaClient({ path: 'http://db:8000' });

  try {
    await chromaClient.getCollection({ name: 'brain-collection' });
  } catch (error) {
    await chromaClient.createCollection({
      name: 'brain-collection',
    });
  }

  this.vectorStore = new Chroma(this.embeddings, {
    url: 'http://db:8000',
    collectionName: 'brain-collection',
  });
  
  console.log('AiService inicializado com sucesso!');
}

async ingest(ingestDto: IngestDto) {
  const { content, filePath, lastModified } = ingestDto;
  
  // Converte a string de data do DTO para um objeto Date
  const newModTime = new Date(lastModified);

  // --- ETAPA 1: VERIFICAR O BANCO ---
  try {
    const existing = await this.vectorStore.similaritySearch(
      ' ', // Query de busca "vazia"
      1,   // Queremos apenas 1 resultado
      { source: filePath }, // O filtro exato
    );

    // Checa se o arquivo já existe no banco
    if (existing && existing.length > 0) {
      const oldModTime = new Date(existing[0].metadata.lastModified as string);

      // --- CASO B: ARQUIVO IDÊNTICO ---
      // Se a data do banco for igual ou mais nova, ignore.
      if (oldModTime >= newModTime) {
        console.log(`IGNORADO (atualizado): ${filePath}`);
        return {
          status: 'ok',
          message: `Arquivo ${filePath} já está atualizado.`,
        };
      }
    }

    // --- CASO A (Novo) ou C (Modificado) ---
    // Se não existe, ou se newModTime > oldModTime,
    // nós executamos o "Delete-then-Add".

    console.log(`Iniciando "Delete-then-Add" para: ${filePath}`);

    // 1. Deletar (não faz nada se não existir)
    await this.vectorStore.delete({
      filter: { source: filePath },
    });

    // 2. Adicionar os novos chunks
    const chunks = await this.textSplitter.createDocuments(
      [content],
      [{ source: filePath, lastModified: newModTime.toISOString() }], // <-- Adicionamos o ModTime
    );

    await this.vectorStore.addDocuments(chunks);

    console.log(`Arquivo ${filePath} RE-INDEXADO com sucesso. ${chunks.length} novos chunks criados.`);

    return {
      status: 'ok',
      message: `Arquivo ${filePath} re-indexado. ${chunks.length} chunks criados.`,
    };
  } catch (error) {
    console.error('Erro durante a ingestão "ModTime Check":', error);
    return {
      status: 'error',
      message: 'Falha ao re-indexar o arquivo.',
    };
  }
}

  async ask(askDto: AskDto) {
    const { query } = askDto;
    console.log(`Iniciando RAG para a query: ${query}`);

    try {
      const promptTemplate = ChatPromptTemplate.fromMessages([
        ['system',
          'Você é um assistente prestativo. Responda a pergunta do usuário usando APENAS o contexto fornecido. ' +
          'Se a resposta não estiver no contexto, diga "Eu não encontrei essa informação nos seus arquivos.".\n' +
          'Seja conciso e direto.\n\n' +
          'Contexto:\n{contexto_formatado}'
        ],
        ['user', 'Pergunta: {query}'],
      ]);

      const retriever = this.vectorStore.asRetriever({
        k: 4,
      });

      const ragChain = RunnableSequence.from([
        {
          contexto_formatado: (input) => retriever.pipe(formatDocumentsAsString).invoke(input.query),
          query: (input) => input.query,
        },
        promptTemplate,
        this.model,
        new StringOutputParser(),
      ]);

      const answer = await ragChain.invoke({ query });

      console.log('Resposta do RAG:', answer);
      return {
        status: 'ok',
        answer,
      };

    } catch (error) {
      console.error('Erro durante o RAG:', error);
      return {
        status: 'error',
        message: 'Falha ao processar a pergunta.',
      };
    }
  }
}
