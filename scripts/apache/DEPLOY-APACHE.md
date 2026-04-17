# Apache Deployment

This project already serves the Web UI from the Go process. Apache should act as a reverse proxy in front of it.

The sample vhost in this directory is configured for:

- domain: `refine.fordivepw.com`
- backend: `http://127.0.0.1:8081`

That backend port matches the current `web_addr` value in `configs/config.json`.

## 1. Enable the required Apache modules

```bash
sudo a2enmod proxy proxy_http headers rewrite ssl
```

## 2. Install the Apache vhost

```bash
sudo cp /home/golang-go/scripts/apache/refine.fordivepw.com.conf /etc/apache2/sites-available/refine.fordivepw.com.conf
sudo a2ensite refine.fordivepw.com.conf
sudo apache2ctl configtest
sudo systemctl reload apache2
```

## 3. Install the systemd service

```bash
sudo cp /home/golang-go/scripts/systemd/refine-monitor.service /etc/systemd/system/refine-monitor.service
sudo systemctl daemon-reload
sudo systemctl enable --now refine-monitor
```

## 4. Check runtime status

```bash
systemctl status refine-monitor
journalctl -u refine-monitor -f
```

## 5. Install the TLS certificate

```bash
sudo apt update
sudo apt install -y certbot python3-certbot-apache
sudo certbot certonly --apache -d refine.fordivepw.com
sudo systemctl reload apache2
```

## Notes

- Ensure DNS for `refine.fordivepw.com` points to this Apache server first.
- If you change `web_addr`, update the `ProxyPass` target to match.
- The service rebuilds the Go binary on each restart by calling `scripts/build.sh`.
