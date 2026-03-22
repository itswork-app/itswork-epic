# PR-0 CHECKLIST

Daftar verifikasi yang WAJIB dibaca setiap kali memulai PR baru:

- [ ] **Check Latency**: Ensure the new code meets the Production Elite low latency standards.
- [ ] **Check Stateless**: Verify that no local storage is used. All state MUST be in Redis/Postgres.
- [ ] **Check gRPC**: Confirm that Go and Python communication respects `CONTRACTS.proto`.
