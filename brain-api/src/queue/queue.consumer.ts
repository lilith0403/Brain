// src/queue/queue.consumer.ts
import { Processor, WorkerHost } from '@nestjs/bullmq';
import { Job } from 'bullmq';
import { AiService } from 'src/ai/ai.service'; 
import { IngestDto } from 'src/ai/dtos/ingest.dto';

@Processor('ingest-queue', {
  concurrency: 1, 
})
export class QueueConsumer extends WorkerHost {
  constructor(private readonly aiService: AiService) {
    super();
  }

  async process(job: Job<IngestDto>): Promise<any> {
    const { filePath } = job.data;
    console.log(`[QueueConsumer] Processando trabalho: ${filePath}`);

    try {
      await this.aiService.ingest(job.data);
    } catch (error) {
      console.error(`[QueueConsumer] Falha ao processar ${filePath}`, error);
      throw error; 
    }
  }
}