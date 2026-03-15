# AGENTS.md

This file provides guidelines for agentic coding assistants working in this repository.

## Build, Lint, and Test Commands

### Frontend (React/Vite)
```bash
cd web
npm install                    # Install dependencies
npm run dev                    # Start dev server (port 3000)
npm run demo                   # Start in demo mode with mock data
npm run build                  # Build for production
npm run preview                # Preview production build
```

### Go Client
```bash
cd client
go test ./...                  # Run all tests
go test -v ./...               # Run tests with verbose output
go test -run TestFunctionName  # Run single test function
go build -o cloud-proxy ./...  # Build binary
go fmt ./...                   # Format all Go files
go vet ./...                   # Run Go vet
```

### Python Server (Aliyun FC)
```bash
cd server
# Uses standard library only, no build steps
python3 -m pytest              # If pytest is available
python3 index.py               # Test locally
```

### Docker
```bash
docker compose up -d --build   # Build and start all services
docker compose logs -f cloud-proxy  # View logs
docker compose down             # Stop services
```

## Code Style Guidelines

### Go (Client)
- **Imports**: Grouped as: standard library, third-party, internal (blank lines between groups)
- **Naming**: PascalCase for exported symbols, camelCase for unexported
- **Formatting**: Use `go fmt ./...` before committing
- **Error handling**: Always check errors, use fmt.Errorf for wrapping
- **Config**: Use TOML via github.com/BurntSushi/toml
- **Logging**: Use github.com/fatih/color for colored console output
- **Concurrency**: Use sync.RWMutex for maps, atomic counters for stats
- **Example**: `proxy/server.go:28-44` shows proper struct definition with atomic fields

### Python (Server)
- **Style**: PEP 8 compliant
- **Libraries**: Standard library only (urllib, socket, ssl)
- **Error handling**: Try-except blocks with specific exception types
- **Functions**: Lowercase with underscores, descriptive names
- **Returns**: Always return dict response with statusCode, headers, body
- **Example**: `server/index.py:11-27` shows proper function structure

### React/JavaScript (Web)
- **Framework**: React 18 with Vite, React Bootstrap 5
- **Components**: Functional components with hooks
- **State**: useState for local state, useEffect for side effects
- **API**: Axios with interceptors for auth tokens
- **Routing**: React Router v6 with protected routes
- **i18n**: react-i18next for internationalization (zh/en)
- **Styling**: Bootstrap classes, no custom CSS preferred
- **Files**: .jsx extension for components, .js for utilities
- **Example**: `web/src/App.jsx:13-21` shows proper ProtectedRoute pattern

## Project Structure

- `client/` - Go proxy client with dashboard
  - `proxy/` - Proxy server implementation
  - `dashboard/` - Web dashboard API handlers
  - `auth/` - JWT authentication with SHA-256 password hashing
  - `config/` - TOML configuration management
  - `cloud/` - Cloud provider integration
- `server/` - Python entry point for Aliyun Function Compute
- `web/` - React frontend for dashboard
- `config/` - Deployment configuration templates
- `scripts/` - Deployment and setup scripts

## Common Patterns

### Authentication
- JWT tokens stored in localStorage as 'jwt_token'
- Password: SHA-256 hash → hex string → Base64 encoding
- Auth header: `Authorization: Bearer <token>`

### Error Handling
- Go: Return errors explicitly, log with color for user-facing messages
- Python: Try-except with mk_response(status, {"error": message})
- React: axios interceptors handle 401 and redirect to /login

### Configuration
- Go: TOML files with struct tags for parsing
- Environment variables via docker-compose.yml
- Default config generation in client/config/config.go

## Notes

- No linter config files present (.eslintrc, .prettierrc)
- TypeScript not used; plain JavaScript with JSX
- Go version: 1.25
- Frontend dev server proxies /api to localhost:8081
- Docker Compose manages multi-container setup