import React, { useState, useEffect, useRef } from 'react';
import { Container, Row, Col, Card, Form, Button, Badge, Alert } from 'react-bootstrap';
import { useTranslation } from 'react-i18next';
import { logsAPI, authAPI, isDemoMode } from '../api';

const Logs = () => {
  const { t } = useTranslation();
  const [logs, setLogs] = useState([]);
  const [filter, setFilter] = useState({ status: '', timeRange: '1h' });
  const [ws, setWs] = useState(null);
  const logsEndRef = useRef(null);

  useEffect(() => {
    fetchHistoryLogs();
    if (!isDemoMode) {
      connectWebSocket();
    }

    let interval;
    if (isDemoMode) {
      interval = setInterval(fetchHistoryLogs, 4000);
    }

    return () => {
      if (ws) ws.close();
      if (interval) clearInterval(interval);
    };
  }, []);

  useEffect(() => {
    logsEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [logs]);

  const connectWebSocket = () => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const token = authAPI.getToken();
    const wsUrl = `${protocol}//${window.location.host}/ws/logs`;
    const websocket = token
      ? new WebSocket(wsUrl, ['bearer', token])
      : new WebSocket(wsUrl);

    websocket.onmessage = (event) => {
      const logEntry = JSON.parse(event.data);
      setLogs(prev => [...prev.slice(-199), logEntry]);
    };

    websocket.onerror = (error) => {
      console.error('WebSocket error:', error);
    };

    setWs(websocket);
  };

  const fetchHistoryLogs = async () => {
    try {
      const response = await logsAPI.getLogs(filter);
      setLogs(response.data || []);
    } catch (err) {
      console.error('Failed to fetch logs:', err);
    }
  };

  const handleFilterChange = async (field, value) => {
    const newFilter = { ...filter, [field]: value };
    setFilter(newFilter);
    try {
      const response = await logsAPI.getLogs(newFilter);
      setLogs(response.data || []);
    } catch (err) {
      console.error('Failed to fetch logs:', err);
    }
  };

  const exportLogs = () => {
    const data = logs.map(log => JSON.stringify(log)).join('\n');
    const blob = new Blob([data], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `logs-${new Date().toISOString()}.json`;
    a.click();
  };

  const getLevelColor = (level) => {
    switch (level?.toLowerCase()) {
      case 'error': return 'danger';
      case 'warn': return 'warning';
      case 'info': return 'info';
      default: return 'secondary';
    }
  };

  return (
    <Container fluid>
      <h2 className="mb-4">
        <i className="material-icons me-2">description</i>
        {t('logs.title')}
      </h2>

      <Card className="mb-4">
        <Card.Body>
          <Row>
            <Col md={3}>
              <Form.Label>{t('logs.filterByStatus')}</Form.Label>
              <Form.Select
                value={filter.status}
                onChange={(e) => handleFilterChange('status', e.target.value)}
              >
                <option value="">All</option>
                <option value="success">Success</option>
                <option value="error">Error</option>
                <option value="warn">Warning</option>
              </Form.Select>
            </Col>
            <Col md={3}>
              <Form.Label>{t('logs.filterByTime')}</Form.Label>
              <Form.Select
                value={filter.timeRange}
                onChange={(e) => handleFilterChange('timeRange', e.target.value)}
              >
                <option value="1h">Last 1 hour</option>
                <option value="6h">Last 6 hours</option>
                <option value="24h">Last 24 hours</option>
                <option value="7d">Last 7 days</option>
              </Form.Select>
            </Col>
            <Col md={3} className="d-flex align-items-end">
              <Button variant="primary" onClick={fetchHistoryLogs}>
                <i className="material-icons me-1">refresh</i>
                {t('common.refresh')}
              </Button>
            </Col>
            <Col md={3} className="d-flex align-items-end">
              <Button variant="outline-primary" onClick={exportLogs}>
                <i className="material-icons me-1">download</i>
                {t('logs.exportLogs')}
              </Button>
            </Col>
          </Row>
        </Card.Body>
      </Card>

      <Card>
        <Card.Header>
          <i className="material-icons me-2">list</i>
          {t('logs.realtimeLogs')}
        </Card.Header>
        <Card.Body>
          {isDemoMode && (
            <Alert variant="info">
              Demo mode uses mocked polling data, so this page will not open a real WebSocket stream.
            </Alert>
          )}
          <div style={{ height: '600px', overflowY: 'auto', backgroundColor: '#f8f9fa', padding: '15px', borderRadius: '5px' }}>
            {logs.length === 0 && (
              <Alert variant="info">{t('common.loading')}</Alert>
            )}
            {logs.map((log, idx) => (
              <div key={idx} className="log-entry">
                <div className="d-flex justify-content-between">
                  <span>
                    <Badge bg={getLevelColor(log.level)}>{log.level}</Badge>
                    <span className="ms-2 text-muted">{log.time}</span>
                  </span>
                  <span className="text-muted">{log.duration}ms</span>
                </div>
                <div className="mt-1">
                  <strong>{log.method}</strong> {log.url}
                </div>
                {log.status >= 400 && (
                  <Alert variant="danger" size="sm" className="mt-1">
                    {log.error}
                  </Alert>
                )}
              </div>
            ))}
            <div ref={logsEndRef} />
          </div>
        </Card.Body>
      </Card>
    </Container>
  );
};

export default Logs;
