import { useEffect, useState } from 'react';
import { Container, Row, Col, Form, Button, Card, Spinner, Badge } from 'react-bootstrap';
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
        <Row className="justify-content-center">
          <Col md={10} lg={8} xl={7}>
            <div className="text-center mb-5">
              <h1 className="display-4 mb-3 fancy-title" style={{fontWeight: 300, letterSpacing: '2px'}}>Vectorpedia</h1>
              <p className="text-muted mb-3" style={{fontSize: '1.1rem', fontWeight: 300}}>
                Vector search across Wikipedia's knowledge base
              </p>
              <div className="text-muted" style={{fontSize: '0.9rem', opacity: 0.8}}>
                {summaryStats?.embeddings?.toLocaleString() ?? '...'} embeddings • {' '}
                {summaryStats?.pages?.toLocaleString() ?? '...'} articles • {' '}
                {summaryStats?.centroids?.toLocaleString() ?? '...'} clusters
              </div>
            </div>
            
            <Form onSubmit={handleSearch}>
              <div className="search-container mb-4" style={{boxShadow: '0 8px 32px rgba(0, 0, 0, 0.3)', borderRadius: '16px'}}>
                <Form.Control
                  type="text"
                  placeholder="Ask a question or search for topics..."
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  className="search-input"
                  size="lg"
                  style={{
                    fontSize: '1.15rem',
                    padding: '18px 24px',
                    backgroundColor: '#1a1a1a',
                    border: '2px solid transparent',
                    borderRadius: '16px 0 0 16px'
                  }}
                />
                <Button 
                  variant="primary" 
                  type="submit" 
                  className="search-button"
                  disabled={isLoading}
                  size="lg"
                  style={{
                    fontSize: '1.1rem',
                    padding: '0 32px',
                    borderRadius: '0 16px 16px 0',
                    background: 'linear-gradient(135deg, #0066ff 0%, #00a2ff 100%)',
                    border: 'none'
                  }}
                >
                  Search
                </Button>
              </div>
              
              <div className="d-flex justify-content-center align-items-center flex-wrap" style={{padding: '0 8px'}}>
                <div className="d-flex gap-4 flex-wrap align-items-center">
                  <Form.Check
                    type="checkbox"
                    id="search-title"
                    label={<span style={{fontSize: '0.95rem', fontWeight: 400}}>Titles</span>}
                    checked={searchTitle}
                    onChange={(e) => setSearchTitle(e.target.checked)}
                    className="mb-2 mb-md-0"
                  />
                  <Form.Check
                    type="checkbox"
                    id="search-summary"
                    label={<span style={{fontSize: '0.95rem', fontWeight: 400}}>Summaries</span>}
                    checked={searchSummary}
                    onChange={(e) => setSearchSummary(e.target.checked)}
                    className="mb-2 mb-md-0"
                  />
                  <Form.Check
                    type="checkbox"
                    id="search-content"
                    label={<span style={{fontSize: '0.95rem', fontWeight: 400}}>Full Content</span>}
                    checked={searchContent}
                    onChange={(e) => setSearchContent(e.target.checked)}
                    className="mb-2 mb-md-0"
                  />
                  <div className="vr mx-2 d-none d-md-block" style={{height: '20px', opacity: 0.3}} />
                  <Form.Check
                    type="checkbox"
                    id="use-rerank"
                    label={<span style={{fontSize: '0.95rem', fontWeight: 400}}>Reranking</span>}
                    checked={useRerank}
                    onChange={(e) => setUseRerank(e.target.checked)}
                    className="mb-2 mb-md-0"
                  />
                </div>
              </div>
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

        <Row className="justify-content-center mt-5">
          <Col md={10} lg={8} xl={7}>
            {isLoading ? (
              <div className="text-center py-5">
                <Spinner animation="border" variant="primary" style={{width: '3rem', height: '3rem'}} />
                <p className="mt-3 text-muted" style={{fontSize: '1.1rem'}}>Searching Wikipedia...</p>
              </div>
            ) : searchResults.length > 0 ? (
              <>
                <div className="mb-4 d-flex align-items-center justify-content-between">
                  <h5 className="m-0" style={{fontWeight: 300}}>
                    Found <span style={{fontWeight: 500}}>{searchResults.length}</span> result{searchResults.length !== 1 ? 's' : ''}
                  </h5>
                  <small className="text-muted">Sorted by relevance</small>
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
                  <div className="mb-3" style={{fontSize: '3rem', opacity: 0.3}}>🔍</div>
                  <h5 className="text-muted mb-3" style={{fontWeight: 300}}>No results found</h5>
                  <p className="text-muted" style={{fontSize: '0.95rem'}}>Try adjusting your search query or enabling more search locations</p>
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
  const sourceColor = result.source === 'title' ? '#0066ff' : 
                       result.source === 'summary' ? '#48bb78' : '#4299e1';
  const sourceLabel = result.source === 'title' ? 'Title Match' : 
                      result.source === 'summary' ? 'Summary Match' : 'Content Match';
  
  return (
    <Card className="mb-3 search-result" style={{
      backgroundColor: '#1a1a1a',
      border: '1px solid rgba(255, 255, 255, 0.05)',
      borderRadius: '12px',
      transition: 'all 0.3s ease',
      overflow: 'hidden'
    }}>
      <Card.Body style={{padding: '20px 24px'}}>
        <div className="d-flex justify-content-between align-items-start mb-3">
          <div style={{flex: 1}}>
            <h5 className="mb-2" style={{fontSize: '1.25rem', fontWeight: 500}}>
              <a 
                href={`${result.uri}`}
                target="_blank" 
                rel="noopener noreferrer"
                style={{
                  color: '#88c4ff',
                  textDecoration: 'none',
                  transition: 'color 0.2s ease'
                }}
                onMouseEnter={(e) => e.currentTarget.style.color = '#a8d4ff'}
                onMouseLeave={(e) => e.currentTarget.style.color = '#88c4ff'}
              >
                {result.title}
              </a>
            </h5>
            <div className="d-flex gap-2 align-items-center mb-2">
              <Badge 
                style={{
                  backgroundColor: sourceColor,
                  fontSize: '0.75rem',
                  fontWeight: 500,
                  padding: '5px 10px',
                  borderRadius: '6px'
                }}
              >
                {sourceLabel}
              </Badge>
              <Badge 
                bg="dark" 
                style={{
                  backgroundColor: 'rgba(255, 255, 255, 0.1)',
                  fontSize: '0.75rem',
                  fontWeight: 500,
                  padding: '5px 10px',
                  borderRadius: '6px'
                }}
              >
                {scorePercent}% match
              </Badge>
            </div>
          </div>
        </div>
        
        {result.summary && (
          <p style={{
            color: '#9ca3af',
            fontSize: '0.95rem',
            lineHeight: '1.7',
            marginBottom: '12px'
          }}>
            {result.summary.length > 280 
              ? result.summary.substring(0, 280) + '...' 
              : result.summary}
          </p>
        )}
        
        <div className="d-flex justify-content-between align-items-center">
          <small>
            <a 
              href={`${result.uri}`}
              target="_blank" 
              rel="noopener noreferrer"
              style={{
                color: '#6b7280',
                fontSize: '0.85rem',
                textDecoration: 'none',
                transition: 'color 0.2s ease'
              }}
              onMouseEnter={(e) => e.currentTarget.style.color = '#9ca3af'}
              onMouseLeave={(e) => e.currentTarget.style.color = '#6b7280'}
            >
              🌐 {result.uri.length > 40 ? result.uri.substring(0, 40) + '...' : result.uri}
            </a>
          </small>
        </div>
      </Card.Body>
    </Card>
  );
}