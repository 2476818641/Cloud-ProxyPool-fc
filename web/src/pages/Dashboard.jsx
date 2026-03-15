import React, { useState, useEffect } from 'react';
import { Container, Row, Col, Card, Spinner, Alert, Button } from 'react-bootstrap';
import { useTranslation } from 'react-i18next';
import { Chart as ChartJS } from 'chart.js/auto';
import { Line, Doughnut } from 'react-chartjs-2';
import { statsAPI } from '../api';

const isHealthyStatus = (status = '') => {
  const normalized = String(status).trim().toLowerCase();
  return normalized === 'healthy' || normalized.startsWith('healthy ');
};

const Dashboard = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [stats, setStats] = useState(null);
  const [history, setHistory] = useState([]);

  useEffect(() => {
    const fetchStats = async () => {
      try {
        const response = await statsAPI.getStats();
        setStats(response.data);
        
        const historyResponse = await statsAPI.getHistory();
        setHistory(historyResponse.data || []);
        
        setError(null);
      } catch (err) {
        setError(err.response?.data?.error || err.message);
      } finally {
        setLoading(false);
      }
    };

    fetchStats();
    const interval = setInterval(fetchStats, 2000);
    return () => clearInterval(interval);
  }, []);

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

  const successRate = stats.total > 0 
    ? ((stats.success / stats.total) * 100).toFixed(1) 
    : 0;

  const qpsData = {
    labels: history.map(h => h.time).slice(-20),
    datasets: [{
      label: t('dashboard.qps'),
      data: history.map(h => h.qps).slice(-20),
      borderColor: 'rgb(75, 192, 192)',
      tension: 0.1
    }]
  };

  const latencyData = {
    labels: history.map(h => h.time).slice(-20),
    datasets: [
      {
        label: t('dashboard.avgLatency'),
        data: history.map(h => h.avgLatency).slice(-20),
        borderColor: 'rgb(54, 162, 235)',
        tension: 0.1
      },
      {
        label: t('dashboard.p95Latency'),
        data: history.map(h => h.p95Latency).slice(-20),
        borderColor: 'rgb(255, 206, 86)',
        tension: 0.1
      }
    ]
  };

  const successRateData = {
    labels: [t('common.success'), t('common.error')],
    datasets: [{
      data: [stats.success, stats.failed],
      backgroundColor: ['#28a745', '#dc3545']
    }]
  };

  return (
    <Container fluid>
      <h2 className="mb-4">
        <i className="material-icons me-2">analytics</i>
        {t('dashboard.title')}
      </h2>

      <Row>
        <Col md={3}>
          <Card className="stat-card">
            <Card.Body>
              <div className="stat-label">{t('dashboard.totalRequests')}</div>
              <div className="stat-value">{stats.total.toLocaleString()}</div>
            </Card.Body>
          </Card>
        </Col>
        <Col md={3}>
          <Card className="stat-card">
            <Card.Body>
              <div className="stat-label">{t('dashboard.successRequests')}</div>
              <div className="stat-value text-success">{stats.success.toLocaleString()}</div>
            </Card.Body>
          </Card>
        </Col>
        <Col md={3}>
          <Card className="stat-card">
            <Card.Body>
              <div className="stat-label">{t('dashboard.failedRequests')}</div>
              <div className="stat-value text-danger">{stats.failed.toLocaleString()}</div>
            </Card.Body>
          </Card>
        </Col>
        <Col md={3}>
          <Card className="stat-card">
            <Card.Body>
              <div className="stat-label">{t('dashboard.successRate')}</div>
              <div className="stat-value">{successRate}%</div>
            </Card.Body>
          </Card>
        </Col>
      </Row>

      <Row className="mt-4">
        <Col md={8}>
          <Card>
            <Card.Header>
              <i className="material-icons me-2">show_chart</i>
              {t('dashboard.qps')}
            </Card.Header>
            <Card.Body>
              <Line data={qpsData} options={{ maintainAspectRatio: false }} />
            </Card.Body>
          </Card>
        </Col>
        <Col md={4}>
          <Card>
            <Card.Header>
              <i className="material-icons me-2">pie_chart</i>
              {t('dashboard.successRate')}
            </Card.Header>
            <Card.Body>
              <Doughnut data={successRateData} />
            </Card.Body>
          </Card>
        </Col>
      </Row>

      <Row className="mt-4">
        <Col md={12}>
          <Card>
            <Card.Header>
              <i className="material-icons me-2">speed</i>
              {t('dashboard.avgLatency')}
            </Card.Header>
            <Card.Body>
              <Line data={latencyData} options={{ maintainAspectRatio: false }} />
            </Card.Body>
          </Card>
        </Col>
      </Row>

      <Row className="mt-4">
        <Col md={12}>
          <Card>
            <Card.Header>
              <i className="material-icons me-2">cloud_queue</i>
              {t('dashboard.nodeStats')}
            </Card.Header>
            <Card.Body>
              <table className="table table-hover">
                <thead>
                  <tr>
                    <th>{t('nodes.url')}</th>
                    <th>{t('nodes.region')}</th>
                    <th>{t('nodes.failures')}</th>
                    <th>{t('nodes.status')}</th>
                    <th>{t('nodes.lastFail')}</th>
                  </tr>
                </thead>
                <tbody>
                  {stats.nodes?.map((node, idx) => (
                    <tr key={idx}>
                      <td>{node.url.substring(0, 50)}...</td>
                      <td>{node.region || extractRegion(node.url)}</td>
                      <td>{node.failure_count}</td>
                      <td className={isHealthyStatus(node.status) ? 'status-healthy' : 'status-melting'}>
                        {node.status}
                      </td>
                      <td>{node.last_fail}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Card.Body>
          </Card>
        </Col>
      </Row>
    </Container>
  );
};

const extractRegion = (url) => {
  const match = url.match(/https?:\/\/[^.]+\.([^.]*)\.aliyuncs\.com/);
  return match ? match[1] : 'Unknown';
};

export default Dashboard;
