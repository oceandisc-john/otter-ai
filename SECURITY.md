# Security Features

## JWT Authentication

Otter-AI now implements proper JWT (JSON Web Token) authentication to replace the previous passphrase-based system.

### How It Works

1. **Authentication Flow**:
   - Client sends passphrase to `/api/v1/auth`
   - Server validates passphrase and generates a JWT token
   - Token is returned to client with expiration time
   - Client includes token in subsequent requests via `Authorization: Bearer <token>` header

2. **Token Properties**:
   - **Expiration**: 24 hours from issuance
   - **Algorithm**: HS256 (HMAC with SHA-256)
   - **Claims**: User ID, issuer, issued time, expiration time
   - **Secret Key**: Configurable via `OTTER_JWT_SECRET` environment variable

3. **Security Benefits**:
   - Tokens expire automatically (24-hour window)
   - Secret key never transmitted over network
   - Signed tokens prevent tampering
   - Stateless authentication (no session storage needed)

### Configuration

```bash
# Set in .env or environment
OTTER_JWT_SECRET=your-secret-key-here
```

**Important**: If `OTTER_JWT_SECRET` is not set, a random secret is generated on startup. This means tokens will be invalidated on server restart. For production, always set a persistent secret key.

### Frontend Integration

The Kelpie UI automatically:
- Stores JWT token (not passphrase) in localStorage
- Includes token in all API requests
- Handles token expiration (401 responses)
- Clears invalid tokens and prompts re-authentication

### Disabling Authentication

To disable authentication entirely, leave `OTTER_HOST_PASSPHRASE` empty or unset. This is useful for:
- Development environments
- Trusted internal networks
- Single-user local deployments

## Rate Limiting

Rate limiting prevents abuse and ensures fair resource usage.

### Configuration

```bash
# Maximum requests per time window
OTTER_RATE_LIMIT=100

# Time window (examples: 30s, 1m, 5m, 1h)
OTTER_RATE_LIMIT_WINDOW=1m
```

### Default Limits

- **Rate**: 100 requests per minute
- **Method**: Sliding window algorithm
- **Granularity**: Per client IP address
- **Headers**: 
  - `X-RateLimit-Limit`: Maximum requests allowed
  - `X-RateLimit-Window`: Time window duration

### How It Works

1. **Client Identification**: Uses client IP address (supports proxy headers)
2. **Sliding Window**: Counts requests within the configured time window
3. **429 Response**: Returns HTTP 429 (Too Many Requests) when limit exceeded
4. **Automatic Cleanup**: Old entries are periodically cleaned from memory

### Proxy Support

Rate limiting correctly identifies clients behind proxies by checking:
1. `X-Forwarded-For` header
2. `X-Real-IP` header
3. `RemoteAddr` (fallback)

### Tuning Recommendations

- **Public APIs**: 60-100 requests/minute
- **Internal APIs**: 200-500 requests/minute
- **Development**: 1000+ requests/minute or disable
- **Heavy operations**: Consider separate limits for specific endpoints

## Best Practices

### Production Deployment

1. **Set JWT Secret**:
   ```bash
   OTTER_JWT_SECRET=$(openssl rand -hex 32)
   ```

2. **Enable Authentication**:
   ```bash
   OTTER_HOST_PASSPHRASE=strong-passphrase-here
   ```

3. **Configure Rate Limits**:
   ```bash
   OTTER_RATE_LIMIT=100
   OTTER_RATE_LIMIT_WINDOW=1m
   ```

4. **Use HTTPS**: Always use TLS in production (configure reverse proxy)

5. **Restrict CORS**: Update `corsMiddleware()` to limit allowed origins

### Token Management

- **Rotation**: Change `OTTER_JWT_SECRET` periodically (invalidates all tokens)
- **Expiration**: Default 24 hours (modify `JWTExpirationTime` constant if needed)
- **Storage**: Frontend stores tokens in localStorage (consider httpOnly cookies for enhanced security)

### Rate Limit Monitoring

Watch for:
- Repeated 429 responses (potential abuse)
- Legitimate users hitting limits (adjust configuration)
- IP addresses with unusual patterns

### Security Headers

Consider adding these headers in your reverse proxy:
```
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
X-XSS-Protection: 1; mode=block
Strict-Transport-Security: max-age=31536000; includeSubDomains
```

## API Response Codes

| Code | Meaning | Description |
|------|---------|-------------|
| 200 | OK | Request successful |
| 400 | Bad Request | Invalid request format |
| 401 | Unauthorized | Missing or invalid token |
| 429 | Too Many Requests | Rate limit exceeded |
| 500 | Internal Server Error | Server-side error |

## Future Enhancements

Planned security improvements:
- [ ] Refresh token mechanism
- [ ] Per-endpoint rate limits
- [ ] Redis-backed rate limiting (for multi-instance deployments)
- [ ] IP allowlist/blocklist
- [ ] Audit logging
- [ ] Two-factor authentication (2FA)
- [ ] API key management for programmatic access
