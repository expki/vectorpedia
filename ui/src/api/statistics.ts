export type StatisticsResponse = {
  servers: {
    servers: Array<{
      url: string;
      is_healthy: boolean;
      total_requests: number;
      active_requests: number;
      gpu_count: number;
    }>;
  };
  database: {
    pages: number;
    embeddings: number;
    centroids: number;
  };
  processing?: {
    pages_per_minute?: number;
  };
};

export async function GetStatistics(): Promise<StatisticsResponse> {
  try {
    const response = await fetch('/api/statistics', {
      method: 'GET',
      headers: {
        'Accept': 'application/json',
      },
    });
    if (!response.ok) {
      throw new Error(response.statusText);
    }

    return await response.json() as StatisticsResponse;
  } catch (err) {
    console.error("Error getting statistics:", err);
    return {
      servers: {
        servers: [],
      },
      database: {
        pages: 0,
        embeddings: 0,
        centroids: 0,
      },
    };
  }
}