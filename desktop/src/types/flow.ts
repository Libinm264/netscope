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

export interface TlsFlowDto {
  recordType: string;        // "ClientHello" | "ServerHello" | "Certificate" | "Alert" | "Finished"
  version: string;           // "TLS 1.2" | "TLS 1.3" etc.
  // ClientHello
  sni?: string;
  cipherSuites: string[];
  hasWeakCipher: boolean;
  // ServerHello
  chosenCipher?: string;
  negotiatedVersion?: string;
  // Certificate
  certCn?: string;
  certSans: string[];
  certExpiry?: string;       // "YYYY-MM-DD"
  certExpired: boolean;
  certIssuer?: string;
  // Alert
  alertLevel?: string;       // "warning" | "fatal"
  alertDescription?: string;
}

export interface IcmpFlowDto {
  icmpType: number;
  icmpCode: number;
  typeStr: string;           // "Echo Request", "Echo Reply", "Destination Unreachable", …
  echoId?: number;
  echoSeq?: number;
  rttMs?: number;            // milliseconds, set on Echo Reply when matching request found
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
  tls?: TlsFlowDto;
  icmp?: IcmpFlowDto;
  arp?: ArpFlowDto;
  tcpStats?: TcpStatsDto;
  rawHex: string;
}

export interface InterfaceDto {
  name: string;
  description: string;
  addresses: string[];
}

export type CaptureStatus = "idle" | "running" | "error";
