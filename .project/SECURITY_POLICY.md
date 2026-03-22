# SECURITY_POLICY

- **Secret Manager**: All sensitive keys, tokens, and credentials MUST be managed through Google Secret Manager. No hardcoded secrets.
- **RLS Database (Row-Level Security)**: Ensure RLS is enabled in Neon DB to securely separate tenant data and prevent unauthorized data access.
