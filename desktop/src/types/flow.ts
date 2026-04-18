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

export interface Http2RequestDto {
  method: string;
  path: string;
  authority: string;
  scheme: string;
  headers: [string, string][];
}

export interface Http2ResponseDto {
  statusCode: number;
  headers: [string, string][];
}

export interface Http2FlowDto {
  streamId: number;
  request?: Http2RequestDto;
  response?: Http2ResponseDto;
  latencyMs?: number;
  grpcService?: string;
  grpcMethod?: string;
  grpcStatus?: number;   // 0 = OK, non-zero = error
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

export interface TlsFlowDto {
  recordType: string;        // "ClientHello" | "ServerHello" | "Certificate" | "Alert" | "Finished"
  version: string;           // "TLS 1.2" | "TLS 1.3" etc.
  sni?: string;
  cipherSuites: string[];
  hasWeakCipher: boolean;
  chosenCipher?: string;
  negotiatedVersion?: string;
  certCn?: string;
  certSans: string[];
  certExpiry?: string;       // "YYYY-MM-DD"
  certExpired: boolean;
  certIssuer?: string;
  alertLevel?: string;       // "warning" | "fatal"
  alertDescription?: string;
}

export interface IcmpFlowDto {
  icmpType: number;
  icmpCode: number;
  typeStr: string;
  echoId?: number;
  echoSeq?: number;
  rttMs?: number;
}

export interface ArpFlowDto {
  operation: string;         // "who-has" | "is-at"
  senderIp: string;
  senderMac: string;
  targetIp: string;
  targetMac: string;
}

export interface TcpStatsDto {
  retransmissions: number;
  outOfOrder: number;
}

export interface GeoInfoDto {
  countryCode: string;       // "US" | "DE" | "??" etc.
  countryName: string;
  city: string;
  asn: number;
  asOrg: string;
}

export interface ThreatInfoDto {
  score: number;             // 0–100
  level: "clean" | "low" | "medium" | "high";
  reasons: string[];
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
  http2?: Http2FlowDto;
  dns?: DnsFlowDto;
  tls?: TlsFlowDto;
  icmp?: IcmpFlowDto;
  arp?: ArpFlowDto;
  tcpStats?: TcpStatsDto;
  geoSrc?: GeoInfoDto;
  geoDst?: GeoInfoDto;
  threat?: ThreatInfoDto;
  source: "local" | "hub";  // where the flow originated
  rawHex: string;
}

export interface InterfaceDto {
  name: string;
  description: string;
  addresses: string[];
}

export type CaptureStatus = "idle" | "running" | "error";
