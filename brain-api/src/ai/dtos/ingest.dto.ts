import { IsString, IsNotEmpty, IsISO8601 } from 'class-validator';

export class IngestDto {
  @IsString()
  @IsNotEmpty()
  filePath: string;

  @IsString()
  @IsNotEmpty()
  content: string;

  @IsISO8601()
  lastModified: string;
}