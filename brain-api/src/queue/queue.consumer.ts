// src/queue/queue.consumer.ts
import { Processor, WorkerHost } from '@nestjs/bullmq';
import { Job } from 'bullmq';
import { AiService } from 'src/ai/ai.service'; // O serviço que faz o trabalho real
import { IngestDto } from 'src/ai/dtos/ingest.dto';

@Processor('ingest-queue', {
  // Limita o processamento para 5 arquivos em paralelo
  // Isso protege o ChromaDB da "enchente"!
  concurrency: 1, 
})
export class QueueConsumer extends WorkerHost {
  constructor(private readonly aiService: AiService) {
    super();
  }

  // Este método será chamado para CADA trabalho na fila
  async process(job: Job<IngestDto>): Promise<any> {
    const { filePath } = job.data;
    console.log(`[QueueConsumer] Processando trabalho: ${filePath}`);

    try {
      // Agora o "trabalhador" chama o AiService, não o controller
      // A lógica do AiService (Delete-then-Add) é a mesma
      await this.aiService.ingest(job.data);
    } catch (error) {
      console.error(`[QueueConsumer] Falha ao processar ${filePath}`, error);
      throw error; // Lança o erro para o Bull saber que falhou
    }
  }
}