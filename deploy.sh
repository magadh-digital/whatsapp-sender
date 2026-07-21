#!/bin/bash
set -e

APP_NAME="notify"
SERVICE_NAME="app-notify"
APP_DIR="/opt/$APP_NAME"
SERVICE_FILE="$SERVICE_NAME.service"

echo "==> Building binary..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $APP_NAME .

echo "==> Copying binary to $APP_DIR..."
sudo mkdir -p $APP_DIR
sudo cp $APP_NAME $APP_DIR/
sudo cp .env $APP_DIR/.env 2>/dev/null || echo "    [!] No .env found, make sure to create $APP_DIR/.env"

echo "==> Setting up systemd service..."
sudo cp $SERVICE_FILE /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable $SERVICE_NAME
sudo systemctl restart $SERVICE_NAME

echo "==> Done! Service status:"
sudo systemctl status $SERVICE_NAME --no-pager

echo ""
echo "Useful commands:"
echo "  systemctl list-units 'app-*' --type=service   # list all app services"
echo "  sudo systemctl status $SERVICE_NAME           # check status"
echo "  sudo journalctl -u $SERVICE_NAME -f           # follow logs"
echo "  sudo systemctl restart $SERVICE_NAME          # restart"
echo "  sudo systemctl stop $SERVICE_NAME             # stop"
