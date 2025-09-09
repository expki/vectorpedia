import { useEffect, useState } from 'react';
import { Container, Row, Col, Form, Button, Card, Spinner, Badge } from 'react-bootstrap';
import globe from '../assets/globe.gif';
import { GetSummaryStats, type SummaryStatsResponse } from '../api/summary';
import { Search, type SearchResult } from '../api/search';

export function SearchPage() {
  const [query, setQuery] = useState<string>('');
  const [searchResults, setSearchResults] = useState<SearchResult[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [summaryStats, setSummaryStats] = useState<SummaryStatsResponse | undefined>(undefined);
  
  // Search options
  const [searchTitle, setSearchTitle] = useState<boolean>(true);
  const [searchSummary, setSearchSummary] = useState<boolean>(false);
  const [searchContent, setSearchContent] = useState<boolean>(false);
  const [useRerank, setUseRerank] = useState<boolean>(false);

  useEffect(() => {
    GetSummaryStats().then((result) => setSummaryStats(result));
  }, []);

  const handleSearch = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!query.trim()) {
      setError('Please enter a search query');
      return;
    }

    if (!searchTitle && !searchSummary && !searchContent) {
      setError('Please select at least one search location');
      return;
    }
    
    setIsLoading(true);
    setError(null);
    
    try {
      const response = await Search({
        query: query,
        locations: {
          title: searchTitle,
          summary: searchSummary,
          content: searchContent,
        },
        rerank: useRerank,
        limit: 20,
      });
      setSearchResults(response.results || []);
    } catch {
      setError('Failed to fetch search results. Please try again.');
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="app-container">
      <Container className="py-5">
        <Row className="justify-content-center mb-4">
          <Col md={10} lg={8}>
            <div className='d-flex flex-row justify-content-center align-items-center gap-2 mb-4'>
              <img src={globe} style={{maxWidth: '4rem'}} />
              <h1 className="m-0 text-center fancy-title">Vectorpedia</h1>
            </div>
            
            <Form onSubmit={handleSearch}>
              <div className="search-container mb-3">
                <Form.Control
                  type="text"
                  placeholder="Search Wikipedia articles..."
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  className="search-input"
                  size="lg"
                />
                <Button 
                  variant="primary" 
                  type="submit" 
                  className="search-button"
                  disabled={isLoading}
                  size="lg"
                >
                  {isLoading ? <Spinner animation="border" size="sm" /> : 'Search'}
                </Button>
              </div>
              
              <Row className="mb-2">
                <Col md={8}>
                  <div className="d-flex gap-3 flex-wrap">
                    <Form.Check
                      type="checkbox"
                      id="search-title"
                      label="Titles"
                      checked={searchTitle}
                      onChange={(e) => setSearchTitle(e.target.checked)}
                    />
                    <Form.Check
                      type="checkbox"
                      id="search-summary"
                      label="Summaries"
                      checked={searchSummary}
                      onChange={(e) => setSearchSummary(e.target.checked)}
                    />
                    <Form.Check
                      type="checkbox"
                      id="search-content"
                      label="Content"
                      checked={searchContent}
                      onChange={(e) => setSearchContent(e.target.checked)}
                    />
                    <Form.Check
                      type="switch"
                      id="use-rerank"
                      label="Reranking"
                      checked={useRerank}
                      onChange={(e) => setUseRerank(e.target.checked)}
                    />
                  </div>
                </Col>
                <Col md={4} className="text-md-end">
                  <small className="text-muted">
                    {summaryStats?.pages?.toLocaleString() ?? '...'} articles • {' '}
                    {summaryStats?.embeddings?.toLocaleString() ?? '...'} embeddings • {' '}
                    {summaryStats?.centroids?.toLocaleString() ?? '...'} clusters
                  </small>
                </Col>
              </Row>
            </Form>
          </Col>
        </Row>

        {error && (
          <Row className="justify-content-center">
            <Col md={10} lg={8}>
              <div className="alert alert-danger">{error}</div>
            </Col>
          </Row>
        )}

        <Row className="justify-content-center">
          <Col md={10} lg={8}>
            {isLoading ? (
              <div className="text-center py-5">
                <Spinner animation="border" variant="primary" />
                <p className="mt-2 text-muted">Searching...</p>
              </div>
            ) : searchResults.length > 0 ? (
              <>
                <div className="mb-3 text-muted">
                  Found {searchResults.length} result{searchResults.length !== 1 ? 's' : ''}
                </div>
                <div className="results-container">
                  {searchResults.map((result) => (
                    <SearchResultCard key={result.page_id} result={result} />
                  ))}
                </div>
              </>
            ) : (
              query && !isLoading && (
                <div className="text-center py-5">
                  <h5 className="text-muted">No results found</h5>
                  <p className="text-muted">Try adjusting your search query or enabling more search locations</p>
                </div>
              )
            )}
          </Col>
        </Row>
      </Container>
    </div>
  );
}

function SearchResultCard({ result }: { result: SearchResult }) {
  const scorePercent = (result.score * 100).toFixed(1);
  const sourceColor = result.source === 'title' ? 'primary' : 
                       result.source === 'summary' ? 'success' : 'info';
  
  return (
    <Card className="mb-3 search-result shadow-sm">
      <Card.Body>
        <div className="d-flex justify-content-between align-items-start mb-2">
          <h5 className="mb-0">
            <a 
              href={`https://en.wikipedia.org/wiki/${encodeURIComponent(result.uri)}`}
              target="_blank" 
              rel="noopener noreferrer"
              className="text-decoration-none"
            >
              {result.title}
            </a>
          </h5>
          <div className="d-flex gap-2 align-items-center">
            <Badge bg={sourceColor} className="text-capitalize">
              {result.source}
            </Badge>
            <Badge bg="secondary">
              {scorePercent}%
            </Badge>
          </div>
        </div>
        
        {result.summary && (
          <Card.Text className="text-muted mb-2">
            {result.summary.length > 300 
              ? result.summary.substring(0, 300) + '...' 
              : result.summary}
          </Card.Text>
        )}
        
        <div className="d-flex justify-content-between align-items-center">
          <small className="text-muted">
            <a 
              href={`https://en.wikipedia.org/wiki/${encodeURIComponent(result.uri)}`}
              target="_blank" 
              rel="noopener noreferrer"
              className="text-muted"
            >
              wikipedia.org/wiki/{result.uri}
            </a>
          </small>
        </div>
      </Card.Body>
    </Card>
  );
}