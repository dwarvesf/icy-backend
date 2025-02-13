# Deploying with Fly.io

## Prerequisites

1. Install Flyctl
   ```bash
   # macOS
   brew install flyctl

   # Other platforms
   curl -L https://fly.io/install.sh | sh
   ```

2. Login to Fly.io
   ```bash
   flyctl auth login
   ```

## Deployment Steps

### 1. Configure Secrets
Create a `.env.prod` file with your application's environment variables. Then import them:
```bash
flyctl secrets import < .env.prod
```

### 2. Initialize Fly.io Configuration
```bash
flyctl launch --ha=false
```
- This command will detect your Dockerfile and create a `fly.toml` configuration file
- Choose a name for your application
- Select the region closest to your primary users

### 3. Set Required Environment Variables
Ensure all necessary environment variables are set:
```bash
# Example (replace with your actual secrets)
flyctl secrets set \
  DATABASE_URL=your_postgres_connection_string \
  ETH_RPC_URL=https://mainnet.infura.io/v3/your_project_id \
  BTC_RPC_URL=your_bitcoin_rpc_endpoint
```

### 4. Deploy the Application
```bash
flyctl deploy --ha=false
```

### 5. Verify Deployment
```bash
flyctl status
flyctl open  # Opens the deployed application in your browser
```

## Additional Fly.io Commands

- Scale your app: 
  ```bash
  flyctl scale count 2  # Run 2 instances
  ```

- View logs: 
  ```bash
  flyctl logs
  ```

- SSH into your running app:
  ```bash
  flyctl ssh console
  ```

## Troubleshooting

- Ensure your `Dockerfile` is correctly configured
- Check that all required environment variables are set
- Verify network and database connectivity
- Review Fly.io documentation for advanced configuration: https://fly.io/docs/

## Notes

- The application uses a Dockerfile for deployment
- Secrets are managed through flyctl
- Ensure your `.env` file is not committed to version control
