const BASE = '/api';

export interface Repository {
  id: string;
  user_id: string;
  name: string;
  full_name: string;
  clone_url: string;
  default_branch: string;
  status: string;
  indexed_at?: string;
  created_at: string;
}

export interface Task {
  id: string;
  repo_id: string;
  type: string;
  status: string;
  progress: number;
  error?: string;
  result?: string;
  created_at: string;
  updated_at: string;
}

export interface Conversation {
  id: string;
  user_id: string;
  repo_id: string;
  title: string;
  created_at: string;
}

export interface Message {
  id: string;
  conv_id: string;
  role: string;
  content: string;
  citations?: Citation[];
  tokens: number;
  created_at: string;
}

export interface Citation {
  file: string;
  line: number;
  content?: string;
  score: number;
}

export interface SSEChunk {
  text: string;
  citations?: Citation[];
}

export interface SSEDone {
  confidence: number;
  tokens: number;
  conv_id: string;
}

async function api(path: string, options?: RequestInit): Promise<any> {
  const token = localStorage.getItem('token');
  const res = await fetch(BASE + path, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options?.headers,
    },
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

// Repo API
export async function listRepos(): Promise<Repository[]> {
  return api('/repos');
}

export async function getRepo(id: string): Promise<Repository> {
  return api(`/repos/${id}`);
}

export async function createRepo(fullName: string): Promise<Repository> {
  return api('/repos', {
    method: 'POST',
    body: JSON.stringify({ full_name: fullName }),
  });
}

export async function deleteRepo(id: string): Promise<void> {
  await api(`/repos/${id}`, { method: 'DELETE' });
}

export interface FileEntry {
  file_path: string;
}

export async function getRepoFiles(id: string): Promise<FileEntry[]> {
  return api(`/repos/${id}/files`);
}

export async function reindexRepo(id: string): Promise<{ status: string }> {
  return api(`/repos/${id}/reindex`, { method: 'POST' });
}

export interface FileContentNode {
  id: string;
  type: string;
  name: string;
  signature: string;
  code: string;
  start_line: number;
  end_line: number;
  language: string;
}

export async function getFileContent(repoId: string, filePath: string): Promise<FileContentNode[]> {
  return api(`/repos/${repoId}/file-content?path=${encodeURIComponent(filePath)}`);
}

// Task API
export async function getTask(id: string): Promise<Task> {
  return api(`/tasks/${id}`);
}

// Conversation API
export async function listConversations(): Promise<Conversation[]> {
  return api('/conversations');
}

export async function getConversation(id: string): Promise<Message[]> {
  return api(`/conversations/${id}`);
}

// Auth API
export async function githubCallback(code: string): Promise<{ token: string; user: any }> {
  const res = await fetch('/auth/github/callback', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code }),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

// Ask API with SSE streaming
export function askStream(
  repoId: string,
  question: string,
  conversationId: string | undefined,
  onChunk: (data: SSEChunk) => void,
  onDone: (data: SSEDone) => void,
  onError: (err: string) => void,
): AbortController {
  const controller = new AbortController();
  const token = localStorage.getItem('token');

  fetch(BASE + '/ask', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify({
      repo_id: repoId,
      question,
      ...(conversationId ? { conversation_id: conversationId } : {}),
    }),
    signal: controller.signal,
  })
    .then(async (response) => {
      if (!response.ok) {
        const text = await response.text();
        onError(`HTTP ${response.status}: ${text}`);
        return;
      }

      const reader = response.body!.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });

        // SSE events are delimited by \n\n
        const events = buffer.split('\n\n');
        // Keep the last (potentially incomplete) part in the buffer
        buffer = events.pop() || '';

        for (const eventBlock of events) {
          if (!eventBlock.trim()) continue;

          const lines = eventBlock.split('\n');
          let eventType = '';
          let data = '';

          for (const line of lines) {
            if (line.startsWith('event: ')) {
              eventType = line.slice(7).trim();
            } else if (line.startsWith('data: ')) {
              data = line.slice(6);
            }
          }

          if (!data) continue;

          try {
            const parsed = JSON.parse(data);
            if (eventType === 'chunk') {
              onChunk(parsed);
            } else if (eventType === 'done') {
              onDone(parsed);
            } else if (eventType === 'error') {
              onError(typeof parsed === 'string' ? parsed : JSON.stringify(parsed));
            } else {
              // Fallback: treat unknown events as chunks
              onChunk(parsed);
            }
          } catch {
            // Skip unparseable data
          }
        }
      }
    })
    .catch((err) => {
      if (err.name !== 'AbortError') {
        onError(err.message);
      }
    });

  return controller;
}
