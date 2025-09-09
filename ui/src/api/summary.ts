export type SummaryStatsResponse = {
  pages: number;
  embeddings: number;
  centroids: number;
};

export async function GetSummaryStats(): Promise<SummaryStatsResponse> {
  try {
    const response = await fetch('/api/stats/summary', {
      method: 'GET',
      headers: {
        'Accept': 'application/json',
      },
    });
    if (!response.ok) {
      throw new Error(response.statusText);
    }

    return await response.json() as SummaryStatsResponse;
  } catch (err) {
    console.error("Error getting summary stats:", err);
    return {
      pages: 6694021,
      embeddings: 0,
      centroids: 0,
    };
  }
}