#!/bin/bash
# ============================================================
#  Secret Vault — Uninstall for Ubuntu/Linux
# ============================================================

set -e

APP_NAME="Secret Vault"
DATA_DIR="$HOME/.secretvault"
DESKTOP_FILE="$HOME/.local/share/applications/secretvault.desktop"
BIN_PATHS=(
    "/usr/local/bin/secretvault"
    "$HOME/.local/bin/secretvault"
    "$HOME/bin/secretvault"
)

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo ""
echo -e "${CYAN}╔══════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║      ${APP_NAME} — Uninstaller            ║${NC}"
echo -e "${CYAN}╚══════════════════════════════════════════╝${NC}"
echo ""

# ── 1. Remove binary ──
echo -e "${YELLOW}[1/3] Removing application binary...${NC}"
removed_bin=false
for bin in "${BIN_PATHS[@]}"; do
    if [ -f "$bin" ]; then
        if [ -w "$bin" ]; then
            rm -f "$bin"
            echo -e "  ${GREEN}✓ Removed: $bin${NC}"
        else
            sudo rm -f "$bin"
            echo -e "  ${GREEN}✓ Removed (sudo): $bin${NC}"
        fi
        removed_bin=true
    fi
done
if [ "$removed_bin" = false ]; then
    echo -e "  ${YELLOW}⚠ No binary found in standard locations${NC}"
fi

# ── 2. Remove desktop entry ──
echo -e "${YELLOW}[2/3] Removing desktop entry...${NC}"
if [ -f "$DESKTOP_FILE" ]; then
    rm -f "$DESKTOP_FILE"
    update-desktop-database "$HOME/.local/share/applications" 2>/dev/null || true
    echo -e "  ${GREEN}✓ Removed: $DESKTOP_FILE${NC}"
else
    echo -e "  ${YELLOW}⚠ No desktop entry found${NC}"
fi

# ── 3. Remove user data ──
echo ""
if [ -d "$DATA_DIR" ]; then
    echo -e "${RED}╔══════════════════════════════════════════════════════╗${NC}"
    echo -e "${RED}║  WARNING: This will permanently delete your vault!  ║${NC}"
    echo -e "${RED}║  All notes, files, and keys will be lost forever.   ║${NC}"
    echo -e "${RED}╚══════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "  Data directory: ${CYAN}$DATA_DIR${NC}"

    # Show vault size
    if command -v du &>/dev/null; then
        size=$(du -sh "$DATA_DIR" 2>/dev/null | cut -f1)
        echo -e "  Total size:     ${CYAN}$size${NC}"
    fi
    echo ""

    read -rp "  Delete all vault data? (y/N): " confirm
    if [[ "$confirm" =~ ^[Yy]$ ]]; then
        rm -rf "$DATA_DIR"
        echo -e "  ${GREEN}✓ Vault data deleted${NC}"
    else
        echo -e "  ${YELLOW}⚠ Vault data kept at: $DATA_DIR${NC}"
    fi
else
    echo -e "${YELLOW}[3/3] No vault data found at $DATA_DIR${NC}"
fi

echo ""
echo -e "${GREEN}✅ ${APP_NAME} has been uninstalled.${NC}"
echo ""
