#!/bin/bash
#
# Generate production secrets for SlabLedger
#
# This script generates cryptographically secure secrets for:
# - SESSION_SECRET (32+ characters)
# - ENCRYPTION_KEY (32+ characters)
# - BACKUP_PASSWORD (32+ characters)
#
# Usage: ./scripts/generate-secrets.sh
#

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Header
echo -e "${GREEN}================================${NC}"
echo -e "${GREEN}SlabLedger - Secrets Generator${NC}"
echo -e "${GREEN}================================${NC}"
echo ""

# Check for openssl
if ! command -v openssl &> /dev/null; then
    echo -e "${RED}ERROR: openssl not found${NC}"
    echo "Please install openssl: sudo apt install openssl"
    exit 1
fi

# Generate secrets
echo -e "${YELLOW}Generating secrets...${NC}"
echo ""

SESSION_SECRET=$(openssl rand -hex 32)
ENCRYPTION_KEY=$(openssl rand -hex 32)
BACKUP_PASSWORD=$(openssl rand -base64 32)

# Display secrets
echo -e "${GREEN}✓ Generated SESSION_SECRET (64 characters)${NC}"
echo "SESSION_SECRET=$SESSION_SECRET"
echo ""

echo -e "${GREEN}✓ Generated ENCRYPTION_KEY (64 characters)${NC}"
echo "ENCRYPTION_KEY=$ENCRYPTION_KEY"
echo ""

echo -e "${GREEN}✓ Generated BACKUP_PASSWORD (44 characters)${NC}"
echo "BACKUP_PASSWORD=$BACKUP_PASSWORD"
echo ""

# Create secrets file
SECRETS_FILE=".secrets_$(date +%Y%m%d_%H%M%S).txt"

cat > "$SECRETS_FILE" <<EOF
# SlabLedger Production Secrets
# Generated: $(date)
# IMPORTANT: Store these securely and NEVER commit to git

# Session secret (for session cookie signing)
SESSION_SECRET=$SESSION_SECRET

# Encryption key (for OAuth token encryption)
ENCRYPTION_KEY=$ENCRYPTION_KEY

# Backup password (for encrypted backups)
BACKUP_PASSWORD=$BACKUP_PASSWORD

# Security Notes:
# 1. Add these to your .env.production file
# 2. Store a copy in a secure password manager (1Password, LastPass, etc.)
# 3. Never share these secrets via email or chat
# 4. Rotate these secrets every 90 days
# 5. Use different secrets for dev/staging/production
# 6. If compromised, rotate immediately and invalidate all sessions
EOF

chmod 600 "$SECRETS_FILE"

echo -e "${GREEN}✓ Secrets saved to: $SECRETS_FILE${NC}"
echo -e "${YELLOW}  File permissions: 600 (owner read/write only)${NC}"
echo ""

# Warnings
echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${RED}⚠ IMPORTANT SECURITY WARNINGS ⚠${NC}"
echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo "1. Store these secrets in a secure password manager"
echo "2. NEVER commit $SECRETS_FILE to git"
echo "3. Delete $SECRETS_FILE after copying to .env.production"
echo "4. Keep a backup copy in a secure location"
echo "5. If ENCRYPTION_KEY is lost, all OAuth tokens become unrecoverable"
echo ""

# .env.production template
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}Next Steps:${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo "1. Add these secrets to your .env.production file:"
echo ""
echo "   # Security Configuration"
echo "   SESSION_SECRET=$SESSION_SECRET"
echo "   ENCRYPTION_KEY=$ENCRYPTION_KEY"
echo ""
echo "2. Set file permissions:"
echo ""
echo "   chmod 600 .env.production"
echo ""
echo "3. Add to .gitignore (if not already):"
echo ""
echo "   echo '.env.production' >> .gitignore"
echo "   echo '.secrets_*.txt' >> .gitignore"
echo ""
echo "4. Store BACKUP_PASSWORD securely:"
echo ""
echo "   echo '$BACKUP_PASSWORD' > /opt/slabledger/.backup_password"
echo "   chmod 400 /opt/slabledger/.backup_password"
echo "   sudo chown backupuser:backupuser /opt/slabledger/.backup_password"
echo ""

# Optional: Validate secrets
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}Validation:${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

SESSION_LEN=${#SESSION_SECRET}
ENCRYPTION_LEN=${#ENCRYPTION_KEY}
BACKUP_LEN=${#BACKUP_PASSWORD}

if [ "$SESSION_LEN" -ge 32 ]; then
    echo -e "${GREEN}✓ SESSION_SECRET length: $SESSION_LEN (>= 32)${NC}"
else
    echo -e "${RED}✗ SESSION_SECRET length: $SESSION_LEN (< 32) - REGENERATE!${NC}"
fi

if [ "$ENCRYPTION_LEN" -ge 32 ]; then
    echo -e "${GREEN}✓ ENCRYPTION_KEY length: $ENCRYPTION_LEN (>= 32)${NC}"
else
    echo -e "${RED}✗ ENCRYPTION_KEY length: $ENCRYPTION_LEN (< 32) - REGENERATE!${NC}"
fi

if [ "$BACKUP_LEN" -ge 32 ]; then
    echo -e "${GREEN}✓ BACKUP_PASSWORD length: $BACKUP_LEN (>= 32)${NC}"
else
    echo -e "${RED}✗ BACKUP_PASSWORD length: $BACKUP_LEN (< 32) - REGENERATE!${NC}"
fi

echo ""
echo -e "${GREEN}✓ Secret generation complete!${NC}"
echo ""

# Cleanup reminder
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}Remember to delete $SECRETS_FILE after use:${NC}"
echo -e "${YELLOW}  shred -u $SECRETS_FILE${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
