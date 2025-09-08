import { useEffect, useState } from 'react';
import { Container, Row, Col, Form, Button, Card, Spinner } from 'react-bootstrap';
import globe from '../assets/globe.gif';
import { GetCount, type CountResponse } from '../api/count';
import { Search, type DocumentSearch } from '../api/search';
import { AI } from '../api/chat';

export function SearchPage() {
  const [searchTerm, setSearchTerm] = useState<string|undefined>(undefined);
  const [searchResults, setSearchResults] = useState<DocumentSearch[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [count, setCount] = useState<CountResponse | undefined>(undefined);

  useEffect(() => {
    GetCount().then((result) => setCount(result));
  }, []);

  const handleSearch = async (e: React.FormEvent) => {
    e.preventDefault();
    if (searchTerm === undefined) setSearchTerm('');
    
    setIsLoading(true);
    setError(null);
    
    try {
      const response = await Search({text: searchTerm ?? '', count: 20, offset: 0});
      setSearchResults(response.documents ?? []);
    } catch {
      setError('Failed to fetch search results. Please try again.');
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="app-container">
      <Container className="py-5">
        <Row className="justify-content-center mb-5">
          <Col md={8} lg={6}>
            <div className='d-flex flex-row justify-content-center align-items-center gap-1'>
              <img src={globe} style={{maxWidth: '5rem'}} />
              <h1 className="m-0 text-center fancy-title">Wikipedia Vector Search 2.0</h1>
            </div>
            
            <Form onSubmit={handleSearch}>
              <div className="search-container">
                <Form.Control
                  type="text"
                  placeholder={`Search query...`}
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className="search-input"
                />
                <Button 
                  variant="primary" 
                  type="submit" 
                  className="search-button"
                  disabled={isLoading}
                >
                  {isLoading ? <Spinner animation="border" size="sm" /> : 'Search'}
                </Button>
              </div>              
              <Form.Label className='text-muted' style={{paddingInline: 5}}>
                Embeddings: {count?.embeddings.toLocaleString() ?? '...'} - Documents: {count?.documents.toLocaleString() ?? '...'} - Centroids: {count?.centroids.toLocaleString() ?? '...'}
              </Form.Label>
            </Form>
          </Col>
        </Row>

        {error && (
          <Row className="justify-content-center">
            <Col md={8} lg={6}>
              <div className="alert alert-danger">{error}</div>
            </Col>
          </Row>
        )}

        <Row className="justify-content-center">
          <Col md={10} lg={8}>
            {searchResults.length > 0 ? (
              <div className="results-container">
                {searchResults.map((result) => (
                  <Result key={result.document_id} query={searchTerm ?? ''} result={result} />
                ))}
              </div>
            ) : (
              !isLoading && searchTerm !== undefined && (
                <p className="text-center text-muted">No results found</p>
              )
            )}
          </Col>
        </Row>
      </Container>
    </div>
  );
}

function Result({ query, result }: { query: string, result: DocumentSearch }) {
  const [summaryEnabled, setSummaryEnabled] = useState<boolean>(false);
  const [summary, setSummary] = useState<string | undefined>(undefined);
  const [loading, setLoading] = useState<boolean>(false);
  
  const handleSummaryToggle = () => {
    const enabled = summaryEnabled;
    setSummaryEnabled(!summaryEnabled);
    if (!enabled && !summary) {
      setLoading(true);
      AI(
        {
          query: query,
          id: result.document_id,
        },
        (text: string) => setSummary(text),
        () => setLoading(false),
      );
    }
  }

  return (
    <Card key={result.document_id} className="mb-3 search-result">
      <Card.Body>
        <Card.Title>
          <Row>
            <Col>
              <a 
                href={result.external_id} 
                target="_blank" 
                rel="noopener noreferrer"
              >
                {result.name}
              </a>
            </Col>
            <Col xs="auto">
              <Button 
                variant={summaryEnabled ? "primary" : "outline-primary"}
                size="sm" 
                className="space-right"
                onClick={() => handleSummaryToggle()}
                disabled={loading}
              >
                AI
              </Button>
            </Col>
          </Row>
          
        </Card.Title>
        <Card.Text>
          {summaryEnabled ? summary : `${(50 * (1 + result.document_similarity)).toFixed(4)}% `}
          <a 
            href={result.external_id} 
            target="_blank" 
            rel="noopener noreferrer"
            style={{fontWeight: 400}}
            hidden={summaryEnabled}
          >
            {result.external_id}
          </a>
        </Card.Text>
      </Card.Body>
    </Card>
  );
}