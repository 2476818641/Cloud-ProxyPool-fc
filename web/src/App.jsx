import React, { useState, useEffect } from 'react';
import { BrowserRouter as Router, Routes, Route, Link, Navigate } from 'react-router-dom';
import { Container, Navbar, Nav, NavDropdown, Button } from 'react-bootstrap';
import { useTranslation } from 'react-i18next';
import { authAPI, isDemoMode } from './api';
import Dashboard from './pages/Dashboard';
import Nodes from './pages/Nodes';
import Config from './pages/Config';
import Logs from './pages/Logs';
import Login from './pages/Login';
import DemoShowcase from './pages/DemoShowcase';
import './App.css';

function ProtectedRoute({ children }) {
  const isAuthenticated = authAPI.isAuthenticated();

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  return children;
}

function App() {
  const { t, i18n } = useTranslation();
  const [theme, setTheme] = useState(localStorage.getItem('theme') || 'light');
  const [isAuthenticated, setIsAuthenticated] = useState(authAPI.isAuthenticated());

  useEffect(() => {
    document.body.setAttribute('data-bs-theme', theme);
  }, [theme]);

  const toggleLanguage = () => {
    const newLang = i18n.language === 'zh' ? 'en' : 'zh';
    i18n.changeLanguage(newLang);
    localStorage.setItem('language', newLang);
  };

  const toggleTheme = () => {
    const newTheme = theme === 'light' ? 'dark' : 'light';
    setTheme(newTheme);
    localStorage.setItem('theme', newTheme);
    document.body.setAttribute('data-bs-theme', newTheme);
  };

  const handleLogout = () => {
    authAPI.logout();
    setIsAuthenticated(false);
    window.location.href = '/login';
  };

  if (!isAuthenticated) {
    return (
      <Router>
        <Routes>
          <Route path="/login" element={<Login />} />
          {isDemoMode && <Route path="/demo" element={<DemoShowcase />} />}
          <Route path="*" element={<Navigate to={isDemoMode ? '/demo' : '/login'} replace />} />
        </Routes>
      </Router>
    );
  }

  return (
    <Router>
      <div className={`App ${theme}`}>
        <Navbar bg="primary" variant="dark" expand="lg" className="mb-4">
          <Container>
            <Navbar.Brand as={Link} to="/">
              <i className="material-icons me-2">cloud</i>
              Cloud ProxyPool
            </Navbar.Brand>
            <Navbar.Toggle aria-controls="basic-navbar-nav" />
            <Navbar.Collapse id="basic-navbar-nav">
              <Nav className="me-auto">
                <Nav.Link as={Link} to="/">
                  <i className="material-icons me-1">dashboard</i>
                  {t('common.dashboard')}
                </Nav.Link>
                <Nav.Link as={Link} to="/nodes">
                  <i className="material-icons me-1">router</i>
                  {t('common.nodes')}
                </Nav.Link>
                <Nav.Link as={Link} to="/config">
                  <i className="material-icons me-1">settings</i>
                  {t('common.config')}
                </Nav.Link>
                <Nav.Link as={Link} to="/logs">
                  <i className="material-icons me-1">description</i>
                  {t('common.logs')}
                </Nav.Link>
                {isDemoMode && (
                  <Nav.Link as={Link} to="/demo">
                    <i className="material-icons me-1">visibility</i>
                    {t('common.demo')}
                  </Nav.Link>
                )}
              </Nav>
              <Nav>
                <NavDropdown title={<i className="material-icons">language</i>} align="end">
                  <NavDropdown.Item onClick={toggleLanguage}>
                    {i18n.language === 'zh' ? 'English' : 'Chinese'}
                  </NavDropdown.Item>
                </NavDropdown>
                <Nav.Item>
                  <Button variant="outline-light" size="sm" onClick={toggleTheme}>
                    <i className="material-icons">{theme === 'light' ? 'dark_mode' : 'light_mode'}</i>
                  </Button>
                </Nav.Item>
                <Nav.Item>
                  <Button variant="outline-light" size="sm" onClick={handleLogout}>
                    <i className="material-icons">logout</i>
                  </Button>
                </Nav.Item>
              </Nav>
            </Navbar.Collapse>
          </Container>
        </Navbar>

        <Container fluid className="main-content">
          <Routes>
            <Route path="/" element={
              <ProtectedRoute>
                <Dashboard />
              </ProtectedRoute>
            } />
            <Route path="/nodes" element={
              <ProtectedRoute>
                <Nodes />
              </ProtectedRoute>
            } />
            <Route path="/config" element={
              <ProtectedRoute>
                <Config />
              </ProtectedRoute>
            } />
            <Route path="/logs" element={
              <ProtectedRoute>
                <Logs />
              </ProtectedRoute>
            } />
            {isDemoMode && <Route path="/demo" element={<DemoShowcase />} />}
            <Route path="/login" element={<Login />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </Container>
      </div>
    </Router>
  );
}

export default App;
