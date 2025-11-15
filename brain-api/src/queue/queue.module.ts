import { Module } from '@nestjs/common';
import { BullModule } from '@nestjs/bullmq';
import { QueueController } from './queue.controller';
import { QueueService } from './queue.service';
import { AiModule } from 'src/ai/ai.module'; // <-- Importe o AiModule
import { QueueConsumer } from './queue.consumer';

@Module({
  imports: [
    // Conecta ao Redis e registra uma fila chamada "ingest-queue"
    // Permite configuração flexível: no Docker usa 'queue', localmente usa 'localhost'
    BullModule.forRoot({
      connection: {
        host: process.env.REDIS_HOST || 'localhost',
        port: process.env.REDIS_PORT ? parseInt(process.env.REDIS_PORT) : 6379,
      },
    }),
    BullModule.registerQueue({
      name: 'ingest-queue',
    }),
    AiModule, // <-- Precisamos do AiService
  ],
  controllers: [QueueController],
  // Adicione o Service e o Consumer
  providers: [QueueService, QueueConsumer], 
})
export class QueueModule {}
