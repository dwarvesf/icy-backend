# ICY Backend

<p align="center">
  <img src="https://img.shields.io/badge/golang-1.23-blue" />
  <img src="https://img.shields.io/badge/strategy-gitflow-%23561D25" />
  <a href="https://github.com/consolelabs/mochi-api/blob/main/LICENSE">
    <img src="https://img.shields.io/badge/license-GNU-blue" />
  </a>
</p>

## Overview

This repository is the official BE services for ICY operations.


## Setup local development environment (pick one of following ways)

### Using DEVBOX

Create isolated shell using devbox

```
make shell
```

###  Using your machine environment

1. Install Golang

2. Install Docker


## How to run source code locally

1. Set up source

Set up infras, install dependencies, etc.

```
make init
```

If you use Devbox, it will be initialized automatically the first time you run `make shell`

2. Set up env

Create a file `.env` with these values:

```
DB_HOST="127.0.0.1"
DB_PORT="25432"
DB_USER="postgres"
DB_PASS="postgres"
DB_NAME="icy_backend_local"
DB_SSL_MODE="disable"
ALLOWED_ORIGINS="*"
ENV=dev
```

3. Run source

```
make dev
```

The service starts with port 3000 as the default
