import React, { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { Container, Row, Col, Card, Badge, Button, Spinner, Alert } from 'react-bootstrap';
import { useTranslation } from 'react-i18next';
import { logsAPI, nodesAPI, statsAPI } from '../api';

const isHealthyStatus = (status = '') => {
  const normalized = String(status).trim().toLowerCase();
  return normalized === 'healthy' || normalized.startsWith('healthy ');
};

const DemoShowcase = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [stats, setStats] = useState(null);
  const [nodes, setNodes] = useState([]);
  const [logs, setLogs] = useState([]);
  const [updatedAt, setUpdatedAt] = useState('');

  const healthyCount = useMemo(() => nodes.filter((item) => isHealthyStatus(item.status)).length, [nodes]);
  const disabledCount = useMemo(() => nodes.filter((item) => item.enabled === false).length, [nodes]);

  const loadDemoData = async () => {
    try {
      setError('');
      const [statsRes, nodesRes, logsRes] = await Promise.all([
        statsAPI.getStats(),
        nodesAPI.getNodes(),
        logsAPI.getLogs({ status: 'error', timeRange: '1h' })
      ]);

      setStats(statsRes.data);
      setNodes(nodesRes.data || []);
      setLogs((logsRes.data || []).slice(-6).reverse());
      setUpdatedAt(new Date().toLocaleTimeString('zh-CN', { hour12: false }));
    } catch (err) {
      setError(err.response?.data?.error || err.message || t('common.error'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadDemoData();
  }, []);

  return (
    <Container fluid className="demo-showcase py-4">
      <Card className="demo-hero mb-4">
        <Card.Body className="d-flex flex-column flex-lg-row justify-content-between align-items-start align-items-lg-center gap-3">
          <div>
            <h2 className="mb-2">{t('demo.title')}</h2>
            <p className="mb-0 text-secondary">{t('demo.subtitle')}</p>
          </div>
          <div className="d-flex flex-wrap gap-2">
            <Button variant="outline-primary" onClick={loadDemoData}>
              <i className="material-icons me-1">refresh</i>
              {t('common.refresh')}
            </Button>
            <Button as={Link} to="/login" variant="primary">
              <i className="material-icons me-1">login</i>
              {t('demo.loginToDashboard')}
            </Button>
          </div>
        </Card.Body>
      </Card>

      {error && <Alert variant="danger">{error}</Alert>}

      {loading ? (
        <Card>
          <Card.Body className="text-center py-5">
            <Spinner animation="border" />
          </Card.Body>
        </Card>
      ) : (
        <>
          <Row className="g-3 mb-4">
            <Col md={6} xl={3}>
              <Card className="stat-card h-100">
                <Card.Body>
                  <div className="stat-label">{t('dashboard.totalRequests')}</div>
                  <div className="stat-value">{stats?.total?.toLocaleString?.() || 0}</div>
                </Card.Body>
              </Card>
            </Col>
            <Col md={6} xl={3}>
              <Card className="stat-card h-100">
                <Card.Body>
                  <div className="stat-label">{t('demo.healthyNodes')}</div>
                  <div className="stat-value text-success">{healthyCount} / {nodes.length}</div>
                </Card.Body>
              </Card>
            </Col>
            <Col md={6} xl={3}>
              <Card className="stat-card h-100">
                <Card.Body>
                  <div className="stat-label">{t('demo.disabledNodes')}</div>
                  <div className="stat-value text-warning">{disabledCount}</div>
                </Card.Body>
              </Card>
            </Col>
            <Col md={6} xl={3}>
              <Card className="stat-card h-100">
                <Card.Body>
                  <div className="stat-label">{t('demo.lastUpdated')}</div>
                  <div className="stat-value fs-4">{updatedAt || '--:--:--'}</div>
                </Card.Body>
              </Card>
            </Col>
          </Row>

          <Row className="g-3">
            <Col lg={7}>
              <Card className="h-100">
                <Card.Header>
                  <i className="material-icons me-2">router</i>
                  {t('demo.nodePreview')}
                </Card.Header>
                <Card.Body>
                  {nodes.map((node) => (
                    <div key={node.url} className="demo-node-item d-flex justify-content-between align-items-center py-2 border-bottom">
                      <div>
                        <div className="fw-semibold">{node.region || t('nodes.region')}</div>
                        <div className="text-muted small">{node.url}</div>
                      </div>
                      <div className="text-end">
                        <Badge bg={isHealthyStatus(node.status) ? 'success' : 'danger'}>
                          {isHealthyStatus(node.status) ? t('nodes.healthy') : t('nodes.melting')}
                        </Badge>
                        <div className="small text-muted mt-1">{node.latency} ms</div>
                      </div>
                    </div>
                  ))}
                </Card.Body>
              </Card>
            </Col>
            <Col lg={5}>
              <Card className="h-100">
                <Card.Header>
                  <i className="material-icons me-2">warning</i>
                  {t('demo.errorPreview')}
                </Card.Header>
                <Card.Body>
                  {logs.length === 0 && <p className="text-muted mb-0">{t('demo.noRecentErrors')}</p>}
                  {logs.map((entry, idx) => (
                    <div key={`${entry.time}-${idx}`} className="demo-log-item py-2 border-bottom">
                      <div className="d-flex justify-content-between">
                        <span className="fw-semibold">{entry.method} {entry.status}</span>
                        <span className="small text-muted">{entry.time}</span>
                      </div>
                      <div className="small text-muted">{entry.url}</div>
                    </div>
                  ))}
                </Card.Body>
              </Card>
            </Col>
          </Row>
        </>
      )}
    </Container>
  );
};

export default DemoShowcase;
