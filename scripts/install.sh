#!/bin/bash

# exit on error
set -e

#do not output commands
set +x

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Configuration
GITHUB_REPO="kagent-dev/tools"
BINARY_NAME="kagent-tools"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# Helper functions
log_info() {
    echo -e "${BLUE}ℹ️  ${NC}$1" >&2
}

log_success() {
    echo -e "${GREEN}✅ ${NC}$1" >&2
}

log_step() {
    echo -e "${CYAN}🔄 ${NC}$1" >&2
}

log_step_complete() {
    # Move cursor up one line and clear it, then print the completed message
    echo -e "\033[1A\033[2K${GREEN}✅ ${NC}$1" >&2
}

log_warn() {
    echo -e "${YELLOW}⚠️  ${NC}$1" >&2
}

log_error() {
    echo -e "${RED}❌ ${NC}$1" >&2
}

log_header() {
    echo -e "\n${BOLD}${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}" >&2
    echo -e "${BOLD}${CYAN}  🚀 kagent-tools Installer${NC}" >&2
    echo -e "${BOLD}${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n" >&2
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Detect OS and architecture
detect_platform() {
    local os
    local arch
    
    # Detect OS
    case "$(uname -s)" in
        Darwin)
            os="darwin"
            ;;
        Linux)
            os="linux"
            ;;
        CYGWIN*|MINGW32*|MSYS*|MINGW*)
            os="windows"
            ;;
        *)
            log_error "Unsupported operating system: $(uname -s)"
            exit 1
            ;;
    esac
    
    # Detect architecture
    case "$(uname -m)" in
        x86_64|amd64)
            arch="amd64"
            ;;
        arm64|aarch64)
            arch="arm64"
            ;;
        armv7l)
            arch="arm"
            ;;
        *)
            log_error "Unsupported architecture: $(uname -m)"
            exit 1
            ;;
    esac
    
    echo "${os}-${arch}"
}

# Get latest release version from GitHub
get_latest_version() {
    log_step "Fetching latest version from GitHub..."
    
    if command_exists curl; then
        local response=$(curl -s "https://api.github.com/repos/${GITHUB_REPO}/releases/latest")
    elif command_exists wget; then
        local response=$(wget -qO- "https://api.github.com/repos/${GITHUB_REPO}/releases/latest")
    else
        log_error "Neither curl nor wget is available. Please install one of them."
        exit 1
    fi
    
    # Extract version using basic tools (avoiding jq dependency)
    local version=$(echo "$response" | grep '"tag_name"' | sed -E 's/.*"tag_name": "([^"]+)".*/\1/')
    
    if [ -z "$version" ]; then
        echo "" >&2  # Add newline before error
        log_error "Failed to get latest version from GitHub API"
        exit 1
    fi
    
    echo "$version"
}

# Download binary
download_binary() {
    local version="$1"
    local platform="$2"
    local download_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/${BINARY_NAME}-${platform}"
    local temp_file="/tmp/${BINARY_NAME}"
    rm -rf "/tmp/${BINARY_NAME}"
    
    log_step "Downloading ${BINARY_NAME} ${BOLD}${version}${NC} for ${BOLD}${platform}${NC} from GitHub..."
    
    if command_exists curl; then
        curl -sL -o "$temp_file" "$download_url"
    elif command_exists wget; then
        wget -q -O "$temp_file" "$download_url"
    else
        log_error "Neither curl nor wget is available. Please install one of them."
        exit 1
    fi
    
    if [ ! -f "$temp_file" ]; then
        echo "" >&2  # Add newline before error
        log_error "Failed to download binary from $download_url"
        exit 1
    fi
    
    echo "$temp_file"
}

# Install binary
install_binary() {
    local temp_file="$1"
    local install_path="${INSTALL_DIR}/${BINARY_NAME}"
    
    log_step "Installing ${BINARY_NAME} to ${BOLD}${install_path}${NC}..."
    
    # Create install directory if it doesn't exist
    if [ ! -d "$INSTALL_DIR" ]; then
        echo -e "\n${BLUE}ℹ️  ${NC}📁 Creating install directory: ${INSTALL_DIR}" >&2
        mkdir -p "$INSTALL_DIR"
    fi

    mv "$temp_file" "$install_path"
    chmod +x "$install_path"
    
    # Cleanup
    rm -f "$temp_file"
}

# Verify installation
verify_installation() {
    log_step "Verifying installation..."
    
    if command_exists "$BINARY_NAME"; then
        local version=$("$BINARY_NAME" --version 2>/dev/null || echo "unknown")
        log_step_complete "Installation verified! ${BINARY_NAME} is available in PATH"
        echo -e "${BLUE}ℹ️  ${NC}🏷️  Version: ${BOLD}${version}${NC}" >&2
    else
        echo "" >&2  # Add newline before warning
        log_warn "${BINARY_NAME} is installed but not in PATH"
        log_info "💡 Add this line to your shell profile (~/.bashrc, ~/.zshrc, etc.):"
        echo -e "${CYAN}   export PATH=\"${INSTALL_DIR}:\$PATH\"${NC}" >&2
    fi
}

# Main installation function
main() {
    log_header
    
    # Check prerequisites
    log_step "Checking prerequisites..."
    if ! command_exists curl && ! command_exists wget; then
        echo "" >&2  # Add newline before error
        log_error "This script requires either curl or wget to be installed."
        exit 1
    fi
    log_step_complete "Prerequisites check passed"
    
    # Detect platform
    log_step "Detecting platform..."
    local platform=$(detect_platform)
    log_step_complete "Platform detected: ${BOLD}${platform}${NC}"
    
    # Get latest version
    local version=$(get_latest_version)
    log_step_complete "Found latest version: ${BOLD}${version}${NC}"
    
    # Download binary
    local temp_file=$(download_binary "$version" "$platform")
    log_step_complete "Binary downloaded successfully"
    
    # Install binary
    install_binary "$temp_file"
    log_step_complete "Binary installed successfully"
    
    # Verify installation
    verify_installation
    
    echo -e "\n${BOLD}${GREEN}🎉 Installation completed successfully!${NC}" >&2
    echo -e "${GREEN}   You can now use '${BOLD}kagent-tools${NC}${GREEN}' command.${NC}\n" >&2
}

# Handle command line arguments
case "${1:-}" in
    -h|--help)
        echo -e "${BOLD}${CYAN}🚀 kagent-tools Installer${NC}"
        echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        echo -e "\n${BOLD}USAGE:${NC}"
        echo -e "  $0 [OPTIONS]"
        echo -e "\n${BOLD}DESCRIPTION:${NC}"
        echo -e "  Install kagent-tools from GitHub releases"
        echo -e "\n${BOLD}OPTIONS:${NC}"
        echo -e "  -h, --help     Show this help message"
        echo -e "\n${BOLD}ENVIRONMENT VARIABLES:${NC}"
        echo -e "  INSTALL_DIR    Installation directory (default: \$HOME/.local/bin)"
        echo -e "\n${BOLD}EXAMPLES:${NC}"
        echo -e "  ${GREEN}$0${NC}                    # Install to \$HOME/.local/bin"
        echo -e "  ${GREEN}INSTALL_DIR=~/bin $0${NC}  # Install to ~/bin"
        echo ""
        exit 0
        ;;
    *)
        main "$@"
        ;;
esac


