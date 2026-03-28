import { QueryClient, QueryFunction } from "@tanstack/react-query";
import { buildApiUrl } from "./api";
import { unwrapApiEnvelope } from "./api-envelope";

async function throwIfResNotOk(res: Response) {
  if (!res.ok) {
    try {
      const body = await res.json();
      const message = body?.message || res.statusText;
      throw new Error(`${res.status}: ${message}`);
    } catch {
      const text = (await res.text()) || res.statusText;
      throw new Error(`${res.status}: ${text}`);
    }
  }
}

export async function apiRequest(
  method: string,
  url: string,
  data?: unknown | undefined,
): Promise<Response> {
  // apiRequest 保持现有导出签名，内部改走 buildApiUrl 以兼容开发态跨域请求。
  const res = await fetch(buildApiUrl(url), {
    method,
    headers: data ? { "Content-Type": "application/json" } : {},
    body: data ? JSON.stringify(data) : undefined,
    credentials: "include",
  });

  await throwIfResNotOk(res);
  return res;
}

type UnauthorizedBehavior = "returnNull" | "throw";
export const getQueryFn = <T,>(options: {
  on401: UnauthorizedBehavior;
}): QueryFunction<T> =>
  async ({ queryKey }) => {
    const res = await fetch(buildApiUrl(queryKey.join("/") as string), {
      credentials: "include",
    });

    if (options.on401 === "returnNull" && res.status === 401) {
      return null as T;
    }

    await throwIfResNotOk(res);
    return unwrapApiEnvelope<unknown>(await res.json()) as T;
  };

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      queryFn: getQueryFn({ on401: "throw" }),
      refetchInterval: false,
      refetchOnWindowFocus: false,
      staleTime: Infinity,
      retry: false,
    },
    mutations: {
      retry: false,
    },
  },
});
