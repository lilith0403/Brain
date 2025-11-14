import { Controller, Post, Body } from '@nestjs/common';
import { QueueService } from './queue.service';
import { IngestDto } from 'src/ai/dtos/ingest.dto'; // Reutilize o DTO

@Controller('queue')
export class QueueController {
  constructor(private readonly queueService: QueueService) {}

  @Post('ingest')
  async addIngestJob(@Body() ingestDto: IngestDto) {
    // Apenas adiciona o trabalho à fila. Não espera.
    await this.queueService.addIngestJob(ingestDto);
    return { status: 'ok', message: 'Trabalho adicionado à fila.' };
  }
}