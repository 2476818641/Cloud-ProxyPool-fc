import React, { useState, useEffect } from 'react';
import { Container, Row, Col, Card, Form, Button, Spinner, Alert, Table } from 'react-bootstrap';
import { useTranslation } from 'react-i18next';
import { configAPI } from '../api';

const MAX_CONFIG_USERS = 1;

const Config = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);
  const [success, setSuccess] = useState(false);
  const [config, setConfig] = useState({
    listenAddr: '',
    socksAddr: '',
    user: '',
    password: '',
    dashboardEnabled: true
  });
  const [users, setUsers] = useState([]);
  const hasReachedUserLimit = users.length >= MAX_CONFIG_USERS;

  useEffect(() => {
    fetchConfig();
  }, []);

  const fetchConfig = async () => {
    try {
      const response = await configAPI.getConfig();
      setConfig(response.data);
      setUsers(response.data.users || []);
      setError(null);
    } catch (err) {
      setError(err.response?.data?.error || err.message);
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setSaving(true);
    setSuccess(false);
    try {
      const primaryUser = users[0] || { username: '', password: '' };
      await configAPI.updateConfig({
        ...config,
        user: primaryUser.username,
        password: primaryUser.password,
        users
      });
      setSuccess(true);
      setError(null);
      setTimeout(() => setSuccess(false), 3000);
    } catch (err) {
      setError(err.response?.data?.error || err.message);
      setSuccess(false);
    } finally {
      setSaving(false);
    }
  };

  const addUser = () => {
    if (hasReachedUserLimit) {
      return;
    }
    setUsers([...users, { username: '', password: '' }]);
  };

  const updateUser = (idx, field, value) => {
    const updated = [...users];
    updated[idx][field] = value;
    setUsers(updated);
  };

  const removeUser = (idx) => {
    setUsers(users.filter((_, i) => i !== idx));
  };

  if (loading) {
    return (
      <Container className="text-center py-5">
        <Spinner animation="border" />
      </Container>
    );
  }

  return (
    <Container fluid>
      <h2 className="mb-4">
        <i className="material-icons me-2">settings</i>
        {t('config.title')}
      </h2>

      {error && <Alert variant="danger" dismissible onClose={() => setError(null)}>{error}</Alert>}
      {success && <Alert variant="success" dismissible onClose={() => setSuccess(false)}>{t('common.success')}</Alert>}

      <Card className="mb-4">
        <Card.Header>
          <i className="material-icons me-2">http</i>
          {t('config.proxySettings')}
        </Card.Header>
        <Card.Body>
          <Form onSubmit={handleSubmit}>
            <Row className="mb-3">
              <Col md={6}>
                <Form.Label>{t('config.listenAddr')}</Form.Label>
                <Form.Control
                  type="text"
                  value={config.listenAddr}
                  onChange={(e) => setConfig({ ...config, listenAddr: e.target.value })}
                />
              </Col>
              <Col md={6}>
                <Form.Label>{t('config.socksAddr')}</Form.Label>
                <Form.Control
                  type="text"
                  value={config.socksAddr}
                  onChange={(e) => setConfig({ ...config, socksAddr: e.target.value })}
                />
              </Col>
            </Row>

            <Row className="mb-3">
              <Col md={6}>
                <Form.Check
                  type="switch"
                  label={t('config.enableDashboard')}
                  checked={config.dashboardEnabled}
                  onChange={(e) => setConfig({ ...config, dashboardEnabled: e.target.checked })}
                />
              </Col>
            </Row>

            <Button type="submit" variant="primary" disabled={saving}>
              {saving ? (
                <><Spinner size="sm" animation="border" /> {t('common.loading')}</>
              ) : (
                <><i className="material-icons me-1">save</i>{t('common.save')}</>
              )}
            </Button>
          </Form>
        </Card.Body>
      </Card>

      <Card className="mb-4">
        <Card.Header>
          <i className="material-icons me-2">lock</i>
          {t('config.authSettings')}
        </Card.Header>
        <Card.Body>
          <Button variant="success" size="sm" onClick={addUser} className="mb-3" disabled={hasReachedUserLimit}>
            <i className="material-icons me-1">add</i>
            {t('config.addUser')}
          </Button>
          <Table responsive>
            <thead>
              <tr>
                <th>{t('config.username')}</th>
                <th>{t('config.password')}</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {users.map((user, idx) => (
                <tr key={idx}>
                  <td>
                    <Form.Control
                      type="text"
                      value={user.username}
                      onChange={(e) => updateUser(idx, 'username', e.target.value)}
                      size="sm"
                    />
                  </td>
                  <td>
                    <Form.Control
                      type="password"
                      value={user.password}
                      onChange={(e) => updateUser(idx, 'password', e.target.value)}
                      size="sm"
                    />
                  </td>
                  <td>
                    <Button variant="danger" size="sm" onClick={() => removeUser(idx)}>
                      <i className="material-icons">delete</i>
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Card.Body>
      </Card>
    </Container>
  );
};

export default Config;
