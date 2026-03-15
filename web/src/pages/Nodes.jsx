import React, { useState, useEffect } from 'react';
import { Container, Row, Col, Card, Button, Spinner, Alert, Badge } from 'react-bootstrap';
import { useTranslation } from 'react-i18next';
import { nodesAPI } from '../api';

const isHealthyStatus = (status = '') => {
  const normalized = String(status).trim().toLowerCase();
  return normalized === 'healthy' || normalized.startsWith('healthy ');
};

const Nodes = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [nodes, setNodes] = useState([]);
  const [grouped, setGrouped] = useState({});
  const [testing, setTesting] = useState({});

  useEffect(() => {
    fetchNodes();
    const interval = setInterval(fetchNodes, 5000);
    return () => clearInterval(interval);
  }, []);

  const fetchNodes = async () => {
    try {
      const response = await nodesAPI.getNodes();
      setNodes(response.data);
      
      const groups = {};
      response.data.forEach(node => {
        const region = node.region || extractRegion(node.url);
        if (!groups[region]) groups[region] = [];
        groups[region].push(node);
      });
      setGrouped(groups);
      
      setError(null);
    } catch (err) {
      setError(err.response?.data?.error || err.message);
    } finally {
      setLoading(false);
    }
  };

  const testLatency = async (url) => {
    setTesting(prev => ({ ...prev, [url]: true }));
    try {
      await nodesAPI.testLatency(url);
      await fetchNodes();
    } catch (err) {
      setError(err.response?.data?.error || err.message);
    } finally {
      setTesting(prev => ({ ...prev, [url]: false }));
    }
  };

const toggleNode = async (url, enabled) => {
  try {
    await nodesAPI.updateNode(url, !enabled);
    await fetchNodes();
  } catch (err) {
    setError(err.response?.data?.error || err.message);
  }
};

  if (loading) {
    return (
      <Container className="text-center py-5">
        <Spinner animation="border" />
      </Container>
    );
  }

  if (error) {
    return (
      <Container className="py-5">
        <Alert variant="danger">{t('common.error')}: {error}</Alert>
      </Container>
    );
  }

  return (
    <Container fluid>
      <div className="d-flex justify-content-between align-items-center mb-4">
        <h2>
          <i className="material-icons me-2">router</i>
          {t('nodes.title')}
        </h2>
        <Button variant="primary" onClick={fetchNodes}>
          <i className="material-icons me-1">refresh</i>
          {t('common.refresh')}
        </Button>
      </div>

      {Object.entries(grouped).map(([region, regionNodes]) => (
        <Card key={region} className="mb-4">
          <Card.Header>
            <i className="material-icons me-2">location_on</i>
            {region} ({regionNodes.length} {t('common.nodes')})
          </Card.Header>
          <Card.Body>
            <Row>
              {regionNodes.map((node, idx) => (
                <Col key={idx} md={6} lg={4} className="mb-3">
                  <Card className="node-card">
                    <Card.Body>
                      <div className="d-flex justify-content-between align-items-start mb-2">
                        <Badge bg={isHealthyStatus(node.status) ? 'success' : 'danger'}>
                          {isHealthyStatus(node.status) ? t('nodes.healthy') : t('nodes.melting')}
                        </Badge>
                        {node.enabled !== false && (
                          <Button 
                            variant="outline-danger" 
                            size="sm"
                            onClick={() => toggleNode(node.url, node.enabled)}
                          >
                            <i className="material-icons">block</i>
                          </Button>
                        )}
                      </div>
                      <div className="mb-2">
                        <small className="text-muted">{t('nodes.url')}:</small><br/>
                        <small>{node.url.substring(0, 40)}...</small>
                      </div>
                      <div className="mb-2">
                        <small className="text-muted">{t('nodes.failures')}:</small>{' '}
                        {node.failure_count}
                      </div>
                      <div className="mb-2">
                        <small className="text-muted">{t('nodes.lastFail')}:</small>{' '}
                        {node.last_fail || '-'}
                      </div>
                      <div className="mb-2">
                        <small className="text-muted">{t('nodes.latency')}:</small>{' '}
                        {node.latency || '-'}ms
                      </div>
                      <Button 
                        variant="primary" 
                        size="sm" 
                        className="w-100"
                        onClick={() => testLatency(node.url)}
                        disabled={testing[node.url]}
                      >
                        {testing[node.url] ? (
                          <><Spinner size="sm" animation="border" /> {t('common.loading')}</>
                        ) : (
                          <><i className="material-icons me-1">speed</i>{t('nodes.testLatency')}</>
                        )}
                      </Button>
                    </Card.Body>
                  </Card>
                </Col>
              ))}
            </Row>
          </Card.Body>
        </Card>
      ))}
    </Container>
  );
};

const extractRegion = (url) => {
  const match = url.match(/https?:\/\/[^.]+\.([^.]*)\.aliyuncs\.com/);
  return match ? match[1] : 'Unknown';
};

export default Nodes;
