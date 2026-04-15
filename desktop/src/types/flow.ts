export interface HttpFlowDto {
  method?: string;
  path?: string;
  host?: string;
  statusCode?: number;
  statusText?: string;
  latencyMs?: number;
  reqHeaders: [string, string][];
  respHeaders: [string, string][];
  reqBodyPreview?: string;
  respBodyPreview?: string;
}

export interface DnsAnswerDto {
  name: string;
  recordType: string;
  ttl: number;
  data: string;
}

export interface DnsFlowDto {
  transactionId: number;
  queryName: string;
  queryType: string;
  isResponse: boolean;
  rcode?: string;
  answers: DnsAnswerDto[];
}

export interface FlowDto {
  id: string;
  timestamp: string;
  timeStr: string;
  srcIp: string;
  dstIp: string;
  srcPort: number;
  dstPort: number;
  protocol: string;
  length: number;
  info: string;
  http?: HttpFlowDto;
  dns?: DnsFlowDto;
  rawHex: string;
}

export interface InterfaceDto {
  name: string;
  description: string;
  addresses: string[];
}

export type CaptureStatus = "idle" | "running" | "error";
