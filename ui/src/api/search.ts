export type SearchRequest = {
    text: string,
    count: number,
    offset: number,
};

export type SearchResponse = {
	documents: Array<DocumentSearch> | null,
}

export type DocumentSearch = {
  name: string,
	external_id: string,
  document: string,
	document_id: number,
	document_similarity: number,
}

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
