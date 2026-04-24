#!/bin/bash
set -e

# ============================================================
# Install.sh - Golang Refine Monitor Full Installation
# Supports: Debian/Ubuntu and CentOS/RHEL
# ============================================================

DOMAIN="logs.pwmoonlight.com"
PROJECT_DIR="/home/golang-go"
LOG_FILE="/home/logs/world2.log"
WEB_ADDR="127.0.0.1:9090"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info()    { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[OK]${NC} $1"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error()   { echo -e "${RED}[ERROR]${NC} $1"; }

# ============================================================
# Root check
# ============================================================
if [ "$(id -u)" -ne 0 ]; then
    log_error "This script must be run as root (use sudo)"
    exit 1
fi

# ============================================================
# OS Detection
# ============================================================
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS=$ID
    OS_FAMILY=$ID_LIKE
else
    log_error "Cannot detect OS"
    exit 1
fi

is_debian() {
    [[ "$OS" == "debian" || "$OS" == "ubuntu" ]] || [[ "$OS_FAMILY" == *"debian"* || "$OS_FAMILY" == *"ubuntu"* ]]
}

is_centos() {
    [[ "$OS" == "centos" || "$OS" == "rhel" || "$OS" == "rocky" || "$OS" == "almalinux" ]] || [[ "$OS_FAMILY" == *"rhel"* || "$OS_FAMILY" == *"centos"* ]]
}

if is_debian; then
    PKG_MGR="apt"
    APACHE_SERVICE="apache2"
    APACHE_DIR="/etc/apache2"
    APACHE_SITES="$APACHE_DIR/sites-available"
    APACHE_LOG_DIR="/var/log/apache2"
    log_info "Detected Debian/Ubuntu system"
elif is_centos; then
    if command -v dnf &>/dev/null; then
        PKG_MGR="dnf"
    else
        PKG_MGR="yum"
    fi
    APACHE_SERVICE="httpd"
    APACHE_DIR="/etc/httpd"
    APACHE_SITES="$APACHE_DIR/conf.d"
    APACHE_LOG_DIR="/var/log/httpd"
    log_info "Detected CentOS/RHEL system"
else
    log_error "Unsupported OS: $OS"
    exit 1
fi

# ============================================================
# Interactive Inputs
# ============================================================
echo -e "${BLUE}================================================${NC}"
echo -e "${BLUE}  Golang Refine Monitor - Installer${NC}"
echo -e "${BLUE}  Domain: $DOMAIN${NC}"
echo -e "${BLUE}  WebUI:  $WEB_ADDR${NC}"
echo -e "${BLUE}================================================${NC}"
echo ""

read -rp "Enter email for Let's Encrypt certificate: " LETSENCRYPT_EMAIL
if [ -z "$LETSENCRYPT_EMAIL" ]; then
    log_error "Email is required for Let's Encrypt"
    exit 1
fi

read -rp "Enable firewall (port 80/443)? [Y/n]: " ENABLE_FIREWALL
ENABLE_FIREWALL=${ENABLE_FIREWALL:-Y}

# ============================================================
# Step 1: Update packages
# ============================================================
log_info "Updating package manager..."
if is_debian; then
    apt update -y
    apt upgrade -y
elif is_centos; then
    $PKG_MGR update -y
fi
log_success "Packages updated"

# ============================================================
# Step 2: Install Apache (skip if already installed)
# ============================================================
if command -v apache2 &>/dev/null || command -v httpd &>/dev/null; then
    log_info "Apache already installed, skipping"
else
    log_info "Installing Apache..."
    if is_debian; then
        apt install -y apache2
    elif is_centos; then
        $PKG_MGR install -y httpd
    fi
    log_success "Apache installed"
fi

# ============================================================
# Step 3: Enable Apache modules
# ============================================================
log_info "Enabling Apache modules..."
if is_debian; then
    a2enmod proxy proxy_http ssl rewrite headers 2>/dev/null || true
    log_success "Apache modules enabled (Debian)"
elif is_centos; then
    $PKG_MGR install -y mod_ssl mod_proxy mod_proxy_http 2>/dev/null || true
    log_success "Apache modules installed (CentOS)"
fi

# ============================================================
# Step 4: Install Go
# ============================================================
if command -v go &>/dev/null; then
    GO_VERSION=$(go version | awk '{print $3}')
    log_info "Go already installed: $GO_VERSION"
else
    log_info "Installing Go..."
    if is_debian; then
        apt install -y golang-go
    elif is_centos; then
        $PKG_MGR install -y golang
    fi
    log_success "Go installed: $(go version)"
fi

# ============================================================
# Step 5: Install Certbot
# ============================================================
if command -v certbot &>/dev/null; then
    log_info "Certbot already installed"
else
    log_info "Installing Certbot..."
    if is_debian; then
        apt install -y certbot python3-certbot-apache
    elif is_centos; then
        if [[ "$OS" == "centos" && "$VERSION_ID" == 8* ]]; then
            $PKG_MGR install -y epel-release
            $PKG_MGR install -y certbot python3-certbot-apache mod_ssl
        else
            $PKG_MGR install -y certbot python3-certbot-apache mod_ssl
        fi
    fi
    log_success "Certbot installed"
fi

# ============================================================
# Step 6: Deploy Apache Virtual Host
# ============================================================
log_info "Deploying Apache virtual host for $DOMAIN..."
APACHE_CONF="$APACHE_SITES/${DOMAIN}.conf"

# Remove existing config if any
rm -f "$APACHE_CONF"

# Remove any old certbot-generated configs
rm -f "$APACHE_DIR/sites-enabled/${DOMAIN}-le-ssl.conf" 2>/dev/null || true
rm -f "$APACHE_SITES/${DOMAIN}-le-ssl.conf" 2>/dev/null || true

cat > "$APACHE_CONF" << APACHE_EOF
<VirtualHost *:80>
    ServerName $DOMAIN

    RewriteEngine On
    RewriteRule ^/?(.*)$ https://$DOMAIN/\$1 [R=301,L]
</VirtualHost>

<IfModule mod_ssl.c>
    <VirtualHost *:443>
        ServerName $DOMAIN

        SSLEngine on
        SSLCertificateFile /etc/letsencrypt/live/$DOMAIN/fullchain.pem
        SSLCertificateKeyFile /etc/letsencrypt/live/$DOMAIN/privkey.pem

        ProxyPreserveHost On
        ProxyRequests Off
        SSLProxyEngine On

        RequestHeader set X-Forwarded-Proto "https"
        RequestHeader set X-Forwarded-Port "443"

        ProxyPass /api/events/stream http://127.0.0.1:9090/api/events/stream retry=0 timeout=600 keepalive=On
        ProxyPassReverse /api/events/stream http://127.0.0.1:9090/api/events/stream

        ProxyPass / http://127.0.0.1:9090/ retry=0 timeout=60 keepalive=On
        ProxyPassReverse / http://127.0.0.1:9090/

        ErrorLog \${APACHE_LOG_DIR}/${DOMAIN}-error.log
        CustomLog \${APACHE_LOG_DIR}/${DOMAIN}-access.log combined
    </VirtualHost>
</IfModule>
APACHE_EOF

# Site will be enabled after SSL certificate is obtained
log_success "Apache virtual host deployed: $APACHE_CONF"

# ============================================================
# Step 7: Build Go binary
# ============================================================
log_info "Building Go binary..."
cd "$PROJECT_DIR"
go build -o "$PROJECT_DIR/monitor" ./cmd/monitor/
log_success "Binary built: $PROJECT_DIR/monitor ($(du -h "$PROJECT_DIR/monitor" | cut -f1))"

# ============================================================
# Step 8: Create config.json
# ============================================================
CONFIG_FILE="$PROJECT_DIR/configs/config.json"
if [ ! -f "$CONFIG_FILE" ]; then
    log_info "Creating config.json from example..."
    cp "$PROJECT_DIR/configs/config.example.json" "$CONFIG_FILE"

    # Update log_file and web_addr
    sed -i "s|\"log_file\": \"/path/to/world2.log\"|\"log_file\": \"$LOG_FILE\"|g" "$CONFIG_FILE"
    sed -i "s|\"web_addr\": \"127.0.0.1:8080\"|\"web_addr\": \"$WEB_ADDR\"|g" "$CONFIG_FILE"
    log_success "Config created: $CONFIG_FILE"
    log_warn "Please edit $CONFIG_FILE to add your Discord webhook URLs"
else
    log_info "Config already exists, updating web_addr..."
    sed -i "s|\"web_addr\": \"127.0.0.1:[0-9]*\"|\"web_addr\": \"$WEB_ADDR\"|g" "$CONFIG_FILE"
    sed -i "s|\"log_file\": \"/path/to/world2.log\"|\"log_file\": \"$LOG_FILE\"|g" "$CONFIG_FILE"
    log_success "Config updated"
fi

# ============================================================
# Step 9: Install systemd service
# ============================================================
log_info "Installing systemd service..."
cat > /etc/systemd/system/monitor.service << SERVICE_EOF
[Unit]
Description=Golang Refine Monitor
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$PROJECT_DIR
Environment=HOME=/root
Environment=GOPATH=/root/go
ExecStartPre=/bin/bash $PROJECT_DIR/scripts/build.sh
ExecStart=$PROJECT_DIR/monitor
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
SERVICE_EOF

systemctl daemon-reload
log_success "Systemd service installed"

# ============================================================
# Step 10: Firewall configuration
# ============================================================
if [[ "$ENABLE_FIREWALL" =~ ^[Yy]$ ]]; then
    log_info "Configuring firewall..."
    if is_debian; then
        if command -v ufw &>/dev/null; then
            ufw allow 80/tcp
            ufw allow 443/tcp
            log_success "UFW rules added (port 80, 443)"
        else
            log_warn "UFW not installed, skipping firewall config"
        fi
    elif is_centos; then
        if command -v firewall-cmd &>/dev/null; then
            firewall-cmd --permanent --add-service=http
            firewall-cmd --permanent --add-service=https
            firewall-cmd --reload
            log_success "Firewalld rules added (http, https)"
        else
            log_warn "firewalld not running, skipping firewall config"
        fi
    fi
else
    log_info "Firewall configuration skipped"
fi

# ============================================================
# Step 11: Enable Apache service
# ============================================================
log_info "Enabling Apache service..."
systemctl enable "$APACHE_SERVICE"
# Don't start yet - SSL certificate is not ready
log_success "Apache service enabled"

# ============================================================
# Step 12: SSL Certificate (Certbot)
# ============================================================
log_info "Requesting SSL certificate for $DOMAIN..."

# Stop Apache to free port 80 for standalone certbot
systemctl stop "$APACHE_SERVICE" 2>/dev/null || true

certbot certonly --standalone \
    --non-interactive \
    --agree-tos \
    --email "$LETSENCRYPT_EMAIL" \
    -d "$DOMAIN" || true

# Verify certificate was obtained
if [ ! -f "/etc/letsencrypt/live/$DOMAIN/fullchain.pem" ]; then
    log_error "SSL certificate was not obtained for $DOMAIN"
    log_info "You can request it manually with:"
    log_info "  certbot certonly --standalone -d $DOMAIN"
    exit 1
fi

log_success "SSL certificate obtained"

# ============================================================
# Step 13: Enable site and start Apache
# ============================================================
log_info "Starting Apache with SSL configuration..."
if is_debian; then
    a2ensite "${DOMAIN}.conf" 2>/dev/null || true
fi

if ! systemctl restart "$APACHE_SERVICE"; then
    log_error "Apache failed to start. Check configuration: $APACHE_CONF"
    exit 1
fi
log_success "Apache started"

# ============================================================
# Step 14: Start monitor service
# ============================================================
log_info "Starting monitor service..."
systemctl enable monitor
systemctl restart monitor
sleep 2
if systemctl is-active --quiet monitor; then
    log_success "Monitor service is running"
else
    log_error "Monitor service failed to start"
    log_info "Check logs: journalctl -u monitor -f"
    exit 1
fi

# ============================================================
# Summary
# ============================================================
echo ""
echo -e "${GREEN}================================================${NC}"
echo -e "${GREEN}  Installation Complete!${NC}"
echo -e "${GREEN}================================================${NC}"
echo -e "  Domain:     https://$DOMAIN"
echo -e "  WebUI:      $WEB_ADDR"
echo -e "  Log File:   $LOG_FILE"
echo -e "  Config:     $CONFIG_FILE"
echo -e "  Service:    systemctl status monitor"
echo -e "  Apache:     systemctl status $APACHE_SERVICE"
echo -e ""
echo -e "${YELLOW}  REMINDER: Edit $CONFIG_FILE to add Discord webhook URLs${NC}"
echo -e "${YELLOW}  Then restart: systemctl restart monitor${NC}"
echo -e "${GREEN}================================================${NC}"
