# Env generated at 2025-12-09T09:16:06+08:00
export ENV_CTIME="2025-12-09T09:16:06+08:00"

# --- Fragment: web_service ---
export SERVICE_PORT="8080"
export SERVICE_HOST="0.0.0.0"
if [ -z "$SERVICE_URL" ]; then
  export SERVICE_URL="http://$SERVICE_HOST:$SERVICE_PORT"
fi


