import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import { Container, Card, Form, Button, Alert } from 'react-bootstrap';
import { useTranslation } from 'react-i18next';
import { authAPI, isDemoMode } from '../api';

const Login = () => {
  const { t } = useTranslation();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const fillDemoCredentials = () => {
    setUsername('demo');
    setPassword('demo12345');
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      const response = await authAPI.login(username, password);
      localStorage.setItem('jwt_token', response.token);
      window.location.href = '/';
    } catch (err) {
      setError(err.response?.data?.error || t('login.error'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Container className="d-flex align-items-center justify-content-center" style={{ minHeight: '100vh' }}>
      <Card style={{ maxWidth: '400px', width: '100%' }}>
        <Card.Body>
          <h3 className="text-center mb-4">
            <i className="material-icons me-2">lock</i>
            {t('login.title')}
          </h3>

          {error && (
            <Alert variant="danger" dismissible onClose={() => setError('')}>
              {error}
            </Alert>
          )}

          {isDemoMode && (
            <Alert variant="info">
              <div className="mb-2">
                {t('login.demoAccount')} <strong>demo</strong> / <strong>demo12345</strong>
              </div>
              <div className="d-flex gap-2">
                <Button size="sm" variant="outline-primary" onClick={fillDemoCredentials}>
                  {t('login.fillDemo')}
                </Button>
                <Button as={Link} size="sm" variant="primary" to="/demo">
                  {t('login.viewDemo')}
                </Button>
              </div>
            </Alert>
          )}

          <Form onSubmit={handleSubmit}>
            <Form.Group className="mb-3">
              <Form.Label>{t('login.username')}</Form.Label>
              <Form.Control
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder={t('login.usernamePlaceholder')}
                required
                autoFocus
              />
            </Form.Group>

            <Form.Group className="mb-4">
              <Form.Label>{t('login.password')}</Form.Label>
              <Form.Control
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder={t('login.passwordPlaceholder')}
                required
                minLength={8}
              />
              <Form.Text className="text-muted">
                {t('login.passwordHint')}
              </Form.Text>
            </Form.Group>

            <Button type="submit" variant="primary" className="w-100" disabled={loading}>
              {loading ? (
                <><span className="spinner-border spinner-border-sm me-2"></span>{t('common.loading')}</>
              ) : (
                <><i className="material-icons me-1">login</i>{t('login.submit')}</>
              )}
            </Button>
          </Form>

          <div className="text-center mt-3 text-muted">
            <small>{t('login.footer')}</small>
          </div>
        </Card.Body>
      </Card>
    </Container>
  );
};

export default Login;
