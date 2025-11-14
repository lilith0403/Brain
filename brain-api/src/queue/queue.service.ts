import { Injectable } from '@nestjs/common';
import { InjectQueue } from '@nestjs/bullmq';
import { Queue } from 'bullmq';
import { IngestDto } from 'src/ai/dtos/ingest.dto';

@Injectable()
export class QueueService {
  constructor(@InjectQueue('ingest-queue') private ingestQueue: Queue) {}

  async addIngestJob(ingestDto: IngestDto) {
    // O 'filePath' será o ID do Job para evitar duplicatas na fila
    await this.ingestQueue.add('ingest', ingestDto, {
      jobId: ingestDto.filePath,
      // Se a API reiniciar, remove jobs com mais de 1h
      removeOnComplete: { age: 3600 }, 
      removeOnFail: { age: 24 * 3600 },
    });
  }
}