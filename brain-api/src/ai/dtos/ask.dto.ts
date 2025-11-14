import { IsString, IsNotEmpty } from 'class-validator';

export class AskDto {
  @IsString()
  @IsNotEmpty()
  query: string;
}