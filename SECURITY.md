# Security Policy

## Authentication Payload Visibility
During local web development, it is normal and expected that the login/register JSON payload (including passwords) is visible in the browser's Developer Tools (Network Tab).

**Do NOT implement custom frontend password encryption.** Frontend-side encryption provides no real security because the encryption logic and keys would live in the browser, making them accessible to any attacker.

## Production Security Requirements
For a production deployment, the following security measures MUST be strictly enforced:
1. **HTTPS/TLS:** All communication between the frontend client and the backend API must occur over a secure HTTPS connection. This encrypts the payload in transit, preventing Man-in-the-Middle (MitM) attacks.
2. **Server-Side Hashing:** Passwords are hashed on the backend using strong cryptographic algorithms (e.g., bcrypt) before being stored in the database.
3. **No Plaintext Logging:** Passwords must never be logged or stored in plaintext on the server.
4. **Generic Error Messages:** Authentication error messages should remain generic (e.g., "Invalid email or password") to prevent user enumeration attacks.
5. **Strong Secrets:** Ensure that `JWT_SECRET` and other sensitive environment variables are strong, randomly generated strings and kept strictly confidential.
