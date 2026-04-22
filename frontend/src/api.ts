const JSON_HEADERS = {
  "Content-Type": "application/json",
};

export async function apiFetch<T>(url: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(url, init);
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Request failed: ${response.status}`);
  }
  return response.json() as Promise<T>;
}

export function authHeaders(token: string): HeadersInit {
  return {
    ...JSON_HEADERS,
    Authorization: `Bearer ${token}`,
  };
}

export function jsonRequest(method: string, body?: unknown, token?: string): RequestInit {
  return {
    method,
    headers: token ? authHeaders(token) : JSON_HEADERS,
    body: body === undefined ? undefined : JSON.stringify(body),
  };
}

