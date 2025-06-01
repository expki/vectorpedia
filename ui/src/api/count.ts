export type CountResponse = {
  embeddings: number,
	documents: number,
  centroids: number,
}

export async function GetCount(): Promise<CountResponse> {
  try {
    const response = await fetch('/api/count', {
      method: 'GET',
      headers: {
        'Accept': 'application/json',
        'Content-Type': 'application/json',
      },
    });
    if (!response.ok) {
      throw new Error(response.statusText);
    }

    return await response.json() as CountResponse;
  } catch (err) {
    console.error("Error getting count:", err);
    return {
      embeddings: 0,
      documents: 0,
      centroids: 0,
    };
  }
}
