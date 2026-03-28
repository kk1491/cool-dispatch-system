// ApiEnvelope 定義前後端約定的統一外層結構。
export interface ApiEnvelope<T> {
  code: string;
  message: string;
  data: T;
}

// isApiEnvelope 判斷 payload 是否符合統一 envelope 格式，供 requestJSON / React Query 共用。
export function isApiEnvelope<T>(payload: unknown): payload is ApiEnvelope<T> {
  return Boolean(
    payload &&
    typeof payload === 'object' &&
    'code' in payload &&
    'message' in payload &&
    'data' in payload
  );
}

// unwrapApiEnvelope 優先回傳 envelope 的 data；若不是 envelope，則保持原 payload，
// 讓前端在過渡期內同時兼容既有裸資料接口與已統一的新接口。
export function unwrapApiEnvelope<T>(payload: unknown): T {
  if (isApiEnvelope<T>(payload)) {
    return payload.data;
  }
  return payload as T;
}
