import { Controller, Post, Body } from '@nestjs/common';
import { AiService } from './ai.service';
import { IngestDto } from './dtos/ingest.dto';
import { AskDto } from './dtos/ask.dto';

@Controller('ai')
export class AiController {
  constructor(private readonly aiService: AiService) {}

  @Post('ingest')
  async ingest(@Body() ingestDto: IngestDto) {
    console.log('Recebido /ingest:', ingestDto.filePath);
    return await this.aiService.ingest(ingestDto);
  }

  @Post('ask')
  ask(@Body() askDto: AskDto) {
    // Por enquanto, só vamos logar e chamar o service
    console.log('Recebido /ask:', askDto.query);
    return this.aiService.ask(askDto);
  }
}