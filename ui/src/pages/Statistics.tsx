import { useEffect, useState } from 'react';
import { Container, Row, Col, Card, Badge, Form, Button, Table, ProgressBar, Spinner, Alert } from 'react-bootstrap';
import { BarChart, Bar, PieChart, Pie, Cell, LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';

interface GPUInfo {
  index: number;
  name: string;
  memory_used_bytes: number;
  memory_total_bytes: number;
  memory_usage_percent: number;
  core_usage_percent: number;
  temperature_celsius: number;
  power_draw_watts: number;
}

interface EndpointStats {
  average_ms: number;
  min_ms: number;
  max_ms: number;
  last_ms: number;
}

interface ServerStats {
  url: string;
  is_healthy: boolean;
  total_requests: number;
  active_requests: number;
  gpu_count: number;
  gpus: GPUInfo[];
  endpoints: {
    chat: EndpointStats;
    embed: EndpointStats;
    rerank: EndpointStats;
    tokenize_chat: EndpointStats;
    tokenize_embed: EndpointStats;
    tokenize_rerank: EndpointStats;
    detokenize_chat: EndpointStats;
    detokenize_embed: EndpointStats;
    detokenize_rerank: EndpointStats;
  };
}

interface ProcessingAverages {
  average_embedding_time_ms: number;
  average_summary_time_ms: number;
  average_insert_time_ms: number;
  average_process_page_time_ms: number;
  average_tokenize_chat_time_ms: number;
  average_tokenize_embed_time_ms: number;
  average_detokenize_chat_time_ms: number;
  average_detokenize_embed_time_ms: number;
  total_embeddings: number;
  total_summaries: number;
  total_inserts: number;
  total_pages_processed: number;
  total_tokenize_chat: number;
  total_tokenize_embed: number;
  total_detokenize_chat: number;
  total_detokenize_embed: number;
  pages_per_minute: number;
}

interface StatisticsData {
  servers: {
    servers: ServerStats[];
  };
  processing: ProcessingAverages;
}

const CHART_COLORS = ['#0088FE', '#00C49F', '#FFBB28', '#FF8042', '#8884D8', '#82CA9D', '#FFC658', '#FF6B6B'];

export function Statistics() {
  const [data, setData] = useState<StatisticsData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedServer, setSelectedServer] = useState<string>('all');
  const [showHealthyOnly, setShowHealthyOnly] = useState(false);
  const [sortBy, setSortBy] = useState<'requests' | 'gpu' | 'memory'>('requests');
  const [refreshInterval, setRefreshInterval] = useState(5000);
  const [autoRefresh, setAutoRefresh] = useState(true);

  const fetchStatistics = async () => {
    try {
      const response = await fetch('/api/statistics');
      if (!response.ok) throw new Error('Failed to fetch statistics');
      const jsonData = await response.json();
      setData(jsonData);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch statistics');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchStatistics();
    
    if (autoRefresh && refreshInterval > 0) {
      const interval = setInterval(fetchStatistics, refreshInterval);
      return () => clearInterval(interval);
    }
  }, [autoRefresh, refreshInterval]);

  if (loading) {
    return (
      <Container className="py-5">
        <div className="text-center">
          <Spinner animation="border" variant="primary" />
          <p className="mt-3">Loading statistics...</p>
        </div>
      </Container>
    );
  }

  if (error || !data) {
    return (
      <Container className="py-5">
        <Alert variant="danger">
          {error || 'No data available'}
          <Button variant="link" onClick={fetchStatistics}>Retry</Button>
        </Alert>
      </Container>
    );
  }

  const filteredServers = data.servers.servers.filter(server => {
    if (showHealthyOnly && !server.is_healthy) return false;
    if (selectedServer !== 'all' && server.url !== selectedServer) return false;
    return true;
  });

  const sortedServers = [...filteredServers].sort((a, b) => {
    switch (sortBy) {
      case 'requests':
        return b.total_requests - a.total_requests;
      case 'gpu':
        return b.gpu_count - a.gpu_count;
      case 'memory':
        const aMemUsage = a.gpus.reduce((sum, gpu) => sum + gpu.memory_usage_percent, 0) / a.gpus.length;
        const bMemUsage = b.gpus.reduce((sum, gpu) => sum + gpu.memory_usage_percent, 0) / b.gpus.length;
        return bMemUsage - aMemUsage;
      default:
        return 0;
    }
  });

  // Prepare chart data
  const serverLoadData = sortedServers.slice(0, 10).map(server => ({
    name: server.url.split('://')[1]?.split(':')[0] || server.url,
    requests: server.total_requests,
    active: server.active_requests,
    avgGpuUsage: server.gpus.reduce((sum, gpu) => sum + gpu.core_usage_percent, 0) / server.gpus.length
  }));

  const gpuDistribution = sortedServers.reduce((acc, server) => {
    server.gpus.forEach(gpu => {
      const name = gpu.name.replace('NVIDIA GeForce ', '');
      acc[name] = (acc[name] || 0) + 1;
    });
    return acc;
  }, {} as Record<string, number>);

  const gpuDistributionData = Object.entries(gpuDistribution).map(([name, count]) => ({
    name,
    value: count
  }));

  const endpointPerformance = ['chat', 'embed', 'tokenize_chat', 'tokenize_embed'].map(endpoint => ({
    endpoint,
    average: sortedServers.reduce((sum, server) => 
      sum + (server.endpoints[endpoint as keyof typeof server.endpoints]?.average_ms || 0), 0
    ) / sortedServers.length,
    min: Math.min(...sortedServers.map(server => 
      server.endpoints[endpoint as keyof typeof server.endpoints]?.min_ms || Infinity
    )),
    max: Math.max(...sortedServers.map(server => 
      server.endpoints[endpoint as keyof typeof server.endpoints]?.max_ms || 0
    ))
  }));

  const totalStats = {
    totalServers: data.servers.servers.length,
    healthyServers: data.servers.servers.filter(s => s.is_healthy).length,
    totalGPUs: data.servers.servers.reduce((sum, s) => sum + s.gpu_count, 0),
    totalRequests: data.servers.servers.reduce((sum, s) => sum + s.total_requests, 0),
    activeRequests: data.servers.servers.reduce((sum, s) => sum + s.active_requests, 0),
  };

  return (
    <Container fluid className="py-4">
      <Row className="mb-4">
        <Col>
          <h1 className="mb-3">Server Statistics Dashboard</h1>
          
          {/* Controls */}
          <Card className="mb-4">
            <Card.Body>
              <Row className="align-items-center">
                <Col md={3}>
                  <Form.Group>
                    <Form.Label>Server Filter</Form.Label>
                    <Form.Select value={selectedServer} onChange={(e) => setSelectedServer(e.target.value)}>
                      <option value="all">All Servers</option>
                      {data.servers.servers.map(server => (
                        <option key={server.url} value={server.url}>{server.url}</option>
                      ))}
                    </Form.Select>
                  </Form.Group>
                </Col>
                <Col md={2}>
                  <Form.Group>
                    <Form.Label>Sort By</Form.Label>
                    <Form.Select value={sortBy} onChange={(e) => setSortBy(e.target.value as any)}>
                      <option value="requests">Total Requests</option>
                      <option value="gpu">GPU Count</option>
                      <option value="memory">Memory Usage</option>
                    </Form.Select>
                  </Form.Group>
                </Col>
                <Col md={2}>
                  <Form.Check 
                    type="switch"
                    label="Healthy Only"
                    checked={showHealthyOnly}
                    onChange={(e) => setShowHealthyOnly(e.target.checked)}
                    className="mt-4"
                  />
                </Col>
                <Col md={2}>
                  <Form.Check 
                    type="switch"
                    label="Auto Refresh"
                    checked={autoRefresh}
                    onChange={(e) => setAutoRefresh(e.target.checked)}
                    className="mt-4"
                  />
                </Col>
                <Col md={2}>
                  <Form.Group>
                    <Form.Label>Refresh (ms)</Form.Label>
                    <Form.Control 
                      type="number" 
                      value={refreshInterval} 
                      onChange={(e) => setRefreshInterval(parseInt(e.target.value))}
                      disabled={!autoRefresh}
                    />
                  </Form.Group>
                </Col>
                <Col md={1}>
                  <Button variant="primary" onClick={fetchStatistics} className="mt-4">
                    Refresh
                  </Button>
                </Col>
              </Row>
            </Card.Body>
          </Card>

          {/* Summary Cards */}
          <Row className="mb-4">
            <Col md={2}>
              <Card className="text-center">
                <Card.Body>
                  <h5>Total Servers</h5>
                  <h2>{totalStats.totalServers}</h2>
                  <Badge bg="success">{totalStats.healthyServers} Healthy</Badge>
                </Card.Body>
              </Card>
            </Col>
            <Col md={2}>
              <Card className="text-center">
                <Card.Body>
                  <h5>Total GPUs</h5>
                  <h2>{totalStats.totalGPUs}</h2>
                </Card.Body>
              </Card>
            </Col>
            <Col md={2}>
              <Card className="text-center">
                <Card.Body>
                  <h5>Total Requests</h5>
                  <h2>{totalStats.totalRequests.toLocaleString()}</h2>
                </Card.Body>
              </Card>
            </Col>
            <Col md={2}>
              <Card className="text-center">
                <Card.Body>
                  <h5>Active Requests</h5>
                  <h2>{totalStats.activeRequests}</h2>
                </Card.Body>
              </Card>
            </Col>
            <Col md={2}>
              <Card className="text-center">
                <Card.Body>
                  <h5>Pages/Min</h5>
                  <h2>{data.processing.pages_per_minute.toFixed(1)}</h2>
                </Card.Body>
              </Card>
            </Col>
            <Col md={2}>
              <Card className="text-center">
                <Card.Body>
                  <h5>Total Embeddings</h5>
                  <h2>{data.processing.total_embeddings.toLocaleString()}</h2>
                </Card.Body>
              </Card>
            </Col>
          </Row>

          {/* Charts */}
          <Row className="mb-4">
            <Col md={6}>
              <Card>
                <Card.Header>Server Load Distribution</Card.Header>
                <Card.Body>
                  <ResponsiveContainer width="100%" height={300}>
                    <BarChart data={serverLoadData}>
                      <CartesianGrid strokeDasharray="3 3" />
                      <XAxis dataKey="name" angle={-45} textAnchor="end" height={100} />
                      <YAxis />
                      <Tooltip />
                      <Legend />
                      <Bar dataKey="requests" fill="#0088FE" name="Total Requests" />
                      <Bar dataKey="active" fill="#00C49F" name="Active Requests" />
                    </BarChart>
                  </ResponsiveContainer>
                </Card.Body>
              </Card>
            </Col>
            <Col md={6}>
              <Card>
                <Card.Header>GPU Distribution</Card.Header>
                <Card.Body>
                  <ResponsiveContainer width="100%" height={300}>
                    <PieChart>
                      <Pie
                        data={gpuDistributionData}
                        cx="50%"
                        cy="50%"
                        labelLine={false}
                        label={({name, value}) => `${name}: ${value}`}
                        outerRadius={80}
                        fill="#8884d8"
                        dataKey="value"
                      >
                        {gpuDistributionData.map((_, index) => (
                          <Cell key={`cell-${index}`} fill={CHART_COLORS[index % CHART_COLORS.length]} />
                        ))}
                      </Pie>
                      <Tooltip />
                    </PieChart>
                  </ResponsiveContainer>
                </Card.Body>
              </Card>
            </Col>
          </Row>

          <Row className="mb-4">
            <Col md={12}>
              <Card>
                <Card.Header>Endpoint Performance (ms)</Card.Header>
                <Card.Body>
                  <ResponsiveContainer width="100%" height={300}>
                    <LineChart data={endpointPerformance}>
                      <CartesianGrid strokeDasharray="3 3" />
                      <XAxis dataKey="endpoint" />
                      <YAxis />
                      <Tooltip />
                      <Legend />
                      <Line type="monotone" dataKey="average" stroke="#8884d8" name="Average" />
                      <Line type="monotone" dataKey="min" stroke="#82ca9d" name="Min" />
                      <Line type="monotone" dataKey="max" stroke="#ff8042" name="Max" />
                    </LineChart>
                  </ResponsiveContainer>
                </Card.Body>
              </Card>
            </Col>
          </Row>

          {/* Server Details Table */}
          <Card>
            <Card.Header>Server Details</Card.Header>
            <Card.Body style={{ overflowX: 'auto' }}>
              <Table striped bordered hover size="sm">
                <thead>
                  <tr>
                    <th>Status</th>
                    <th>Server</th>
                    <th>GPUs</th>
                    <th>Total Requests</th>
                    <th>Active</th>
                    <th>Avg GPU Usage</th>
                    <th>Avg Memory</th>
                    <th>Avg Temp</th>
                    <th>Total Power</th>
                    <th>Chat Avg (ms)</th>
                    <th>Embed Avg (ms)</th>
                  </tr>
                </thead>
                <tbody>
                  {sortedServers.map(server => {
                    const avgGpuUsage = server.gpus.reduce((sum, gpu) => sum + gpu.core_usage_percent, 0) / server.gpus.length;
                    const avgMemUsage = server.gpus.reduce((sum, gpu) => sum + gpu.memory_usage_percent, 0) / server.gpus.length;
                    const avgTemp = server.gpus.reduce((sum, gpu) => sum + gpu.temperature_celsius, 0) / server.gpus.length;
                    const totalPower = server.gpus.reduce((sum, gpu) => sum + gpu.power_draw_watts, 0);
                    
                    return (
                      <tr key={server.url}>
                        <td>
                          <Badge bg={server.is_healthy ? 'success' : 'danger'}>
                            {server.is_healthy ? 'Healthy' : 'Unhealthy'}
                          </Badge>
                        </td>
                        <td style={{ fontSize: '0.85em' }}>{server.url}</td>
                        <td>{server.gpu_count}</td>
                        <td>{server.total_requests.toLocaleString()}</td>
                        <td>
                          <Badge bg={server.active_requests > 50 ? 'danger' : server.active_requests > 20 ? 'warning' : 'success'}>
                            {server.active_requests}
                          </Badge>
                        </td>
                        <td>
                          <ProgressBar 
                            now={avgGpuUsage} 
                            label={`${avgGpuUsage.toFixed(0)}%`}
                            variant={avgGpuUsage > 80 ? 'danger' : avgGpuUsage > 50 ? 'warning' : 'success'}
                          />
                        </td>
                        <td>
                          <ProgressBar 
                            now={avgMemUsage} 
                            label={`${avgMemUsage.toFixed(0)}%`}
                            variant={avgMemUsage > 80 ? 'danger' : avgMemUsage > 50 ? 'warning' : 'success'}
                          />
                        </td>
                        <td>
                          <Badge bg={avgTemp > 70 ? 'danger' : avgTemp > 60 ? 'warning' : 'success'}>
                            {avgTemp.toFixed(0)}°C
                          </Badge>
                        </td>
                        <td>{totalPower}W</td>
                        <td>{server.endpoints.chat.average_ms.toFixed(0)}</td>
                        <td>{server.endpoints.embed.average_ms.toFixed(0)}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </Table>
            </Card.Body>
          </Card>

          {/* Processing Statistics */}
          <Card className="mt-4">
            <Card.Header>Processing Statistics</Card.Header>
            <Card.Body>
              <Row>
                <Col md={3}>
                  <h6>Embeddings</h6>
                  <p>Total: {data.processing.total_embeddings.toLocaleString()}</p>
                  <p>Avg Time: {data.processing.average_embedding_time_ms.toFixed(2)}ms</p>
                </Col>
                <Col md={3}>
                  <h6>Summaries</h6>
                  <p>Total: {data.processing.total_summaries.toLocaleString()}</p>
                  <p>Avg Time: {data.processing.average_summary_time_ms.toFixed(2)}ms</p>
                </Col>
                <Col md={3}>
                  <h6>Pages Processed</h6>
                  <p>Total: {data.processing.total_pages_processed.toLocaleString()}</p>
                  <p>Avg Time: {data.processing.average_process_page_time_ms.toFixed(2)}ms</p>
                </Col>
                <Col md={3}>
                  <h6>Tokenization</h6>
                  <p>Chat: {data.processing.total_tokenize_chat.toLocaleString()}</p>
                  <p>Embed: {data.processing.total_tokenize_embed.toLocaleString()}</p>
                </Col>
              </Row>
            </Card.Body>
          </Card>
        </Col>
      </Row>
    </Container>
  );
}