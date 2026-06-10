---
title: First login
description: Bootstrap your first admin account with the bootstrap token.
---

## Bootstrap admin access

After starting goloom, use the bootstrap token configured in your environment:

```bash
BOOTSTRAP_ADMIN_TOKEN=replace-with-a-strong-bootstrap-token
```

1. Open goloom in your browser (default [http://localhost:8080](http://localhost:8080)).
2. Go to **Settings**.
3. Enter your `BOOTSTRAP_ADMIN_TOKEN` to claim the first admin account.

## After first login

Once bootstrap access is established:

1. Create normal API tokens for day-to-day use and automation.
2. Rotate or remove the bootstrap token and other bootstrap secrets.
3. Configure OIDC if you want browser sign-in via an identity provider.

## Security notes

- Set a strong `ENCRYPTION_KEY` before running in production.
- Treat the bootstrap token as a one-time secret — revoke it after setup.
- API tokens are stored as hashes; only the token value shown at creation time can be used.
