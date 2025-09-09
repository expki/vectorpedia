export type SearchRequest = {
  query: string;
  locations: {
    title: boolean;
    summary: boolean;
    content: boolean;
  };
  rerank: boolean;
  limit?: number;
};

export type SearchResponse = {
  results: SearchResult[];
  count: number;
};

export type SearchResult = {
  page_id: number;
  uri: string;
  title: string;
  summary?: string;
  score: number;
  source: string;
};

export async function Search(request: SearchRequest): Promise<SearchResponse> {
  try {
    const response = await fetch('/api/search', {
      method: 'POST',
      headers: {
        'Accept': 'application/json',
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(request),
    });
    if (!response.ok) {
      throw new Error(response.statusText);
    }

    return await response.json() as SearchResponse;
  } catch (err) {
    console.error("Error searching:", err);
    throw err;
  }
}