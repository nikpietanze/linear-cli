# 🤖 Linear CLI for AI Agents

**The fastest way for AI agents to create, manage, and maintain Linear issues programmatically.**

[![Go Version](https://img.shields.io/badge/Go-1.22+-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![AI-Optimized](https://img.shields.io/badge/AI-Optimized-green.svg)](https://github.com/nikpietanze/linear-cli)

## 🎯 **Built for AI Agents**

This CLI is specifically designed for AI agents and automation workflows that need to interact with Linear. Unlike traditional CLIs that require multiple commands and manual setup, this tool provides **single-command issue creation** with **automatic template discovery** and **intelligent caching**.

### **Perfect for:**
- 🤖 **AI Assistants** creating issues from conversations
- 🔄 **Automation Workflows** that need structured issue creation
- 🚀 **CI/CD Pipelines** generating issues from code analysis
- 📊 **Monitoring Systems** creating incidents automatically
- 🛠️ **Development Tools** that integrate with Linear

---

## ⚡ **Quick Start for AI Agents**

### 1. **Authentication** (One-time setup)
```bash
linear-cli auth login
```

### 2. **Create Issues Instantly** (Single command)
```bash
# AI agents can create fully structured issues in one command
linear-cli issues create --team ENG --template "Feature Template" --title "Add dark mode" \
  --sections Summary="Implement dark theme toggle for better UX" \
  --sections Context="Users requested low-light interface option" \
  --sections Requirements="Theme switcher, persistent preference, all components" \
  --sections "Definition of Done"="Dark mode works across entire application"
```

### 3. **That's it!** ✨
- ✅ Template automatically discovered and applied
- ✅ All sections filled with provided content  
- ✅ Issue created with proper Linear formatting
- ✅ No temporary files or cleanup needed

---

## 🚀 **Key Features for AI Agents**

### **🔄 Zero-Configuration Workflow**
- **Auto-Discovery**: Templates are automatically synced when needed
- **Intelligent Caching**: Local template storage for fast access
- **No Setup Required**: Works immediately after authentication

### **📋 Template-Driven Issue Creation**
- **Server-Side Templates**: Uses Linear's native template system
- **Dynamic Section Filling**: Adapts to any team's template structure
- **Structured Content**: Maintains Linear's formatting and organization

### **🤖 AI-Optimized Interface**
- **Single Command Creation**: No multi-step workflows
- **Batch Operations**: Create multiple issues efficiently
- **JSON Output**: Perfect for programmatic parsing
- **Error Handling**: Clear, actionable error messages

### **🛡️ Production-Ready**
- **No Delete Operations**: Safe for production environments
- **Rate Limiting**: Respects Linear's API limits
- **Comprehensive Logging**: Full audit trail
- **Offline Capability**: Works with cached templates

---

## 📖 **AI Agent Examples**

### **Discover Available Templates**
```bash
# Get all templates for a team
linear-cli issues template structure --team ENG

# Get specific template structure
linear-cli issues template structure --team ENG --template "Bug Template"
```

### **Create Different Issue Types**
```bash
# Feature Request
linear-cli issues create --team ENG --template "Feature Template" --title "Add user search" \
  --sections Summary="Implement user search functionality" \
  --sections Context="Users need to find other team members quickly"

# Bug Report  
linear-cli issues create --team ENG --template "Bug Template" --title "Login timeout issue" \
  --sections Summary="Users getting logged out unexpectedly" \
  --sections "Steps to Reproduce"="1. Login 2. Wait 5 minutes 3. Session expires"

# Spike/Research
linear-cli issues create --team ENG --template "Spike Template" --title "Evaluate React 18" \
  --sections Goal="Assess React 18 migration impact" \
  --sections Scope="Performance, breaking changes, timeline"
```

### **JSON Output for Programmatic Use**
```bash
# Get structured response for further processing
linear-cli --json issues create --team ENG --template "Feature Template" --title "API endpoint" \
  --sections Summary="New REST endpoint for user data"
```

---

## 🛠️ **Installation**

### **Option 1: Homebrew (Recommended)**
```bash
brew install nikpietanze/tap/linear-cli
```

### **Option 2: Download Binary**
```bash
# Download latest release for your platform
curl -L -o linear-cli.tar.gz \
  https://github.com/nikpietanze/linear-cli/releases/latest/download/linear-cli_linux_amd64.tar.gz
tar -xzf linear-cli.tar.gz
chmod +x linear-cli
mv linear-cli /usr/local/bin/
```

### **Option 3: Build from Source**
```bash
git clone https://github.com/nikpietanze/linear-cli.git
cd linear-cli
go install .
```

---

## 🔧 **Configuration**

### **Authentication**
```bash
# Interactive login (stores token securely)
linear-cli auth login

# Or set environment variable
export LINEAR_API_KEY="your_api_key_here"

# Verify authentication
linear-cli auth status
```

### **Template Management**
```bash
# Sync templates for a team (automatic when needed)
linear-cli templates sync --team ENG

# View cached templates
linear-cli templates list --team ENG

# Check sync status
linear-cli templates status
```

---

## 📚 **Advanced Usage**

### **Human-Friendly Interactive Mode**
```bash
# Interactive walkthrough for human users
linear-cli issues create --team ENG
```

### **Batch Operations**
```bash
# Create multiple issues from a script
for feature in "search" "filters" "pagination"; do
  linear-cli issues create --team ENG --template "Feature Template" \
    --title "Add $feature functionality" \
    --sections Summary="Implement $feature for better UX"
done
```

### **CI/CD Integration**
```bash
# In your GitHub Actions or CI pipeline
- name: Create Linear issue for failed deployment
  run: |
    linear-cli issues create --team DEVOPS --template "Incident Template" \
      --title "Deployment failed: ${{ github.sha }}" \
      --sections Summary="Deployment pipeline failed" \
      --sections Context="Branch: ${{ github.ref }}, Commit: ${{ github.sha }}"
```

---

## 🤝 **Why This CLI?**

### **Compared to Linear's Official CLI:**
- ✅ **AI-Optimized**: Single-command issue creation vs multi-step workflows
- ✅ **Template-Aware**: Automatic template discovery and application
- ✅ **Batch-Friendly**: Designed for automation and scripting
- ✅ **Zero-Config**: Works immediately after authentication

### **Compared to Direct API Calls:**
- ✅ **Simplified**: No need to manage GraphQL queries
- ✅ **Template Integration**: Automatic server-side template application
- ✅ **Error Handling**: Human-readable error messages
- ✅ **Caching**: Intelligent local template storage

---

## 🔒 **Security & Safety**

- **No Delete Operations**: CLI cannot delete issues or projects
- **Read-Only by Default**: Most operations are read-only
- **Secure Token Storage**: API keys stored with proper permissions
- **Audit Trail**: All operations are logged

---

## 🚀 **Roadmap**

- [ ] **Comment Management**: Create and update issue comments
- [ ] **Bulk Operations**: Import/export issues in batch
- [ ] **Webhook Integration**: Real-time issue synchronization
- [ ] **Custom Templates**: Support for local template definitions
- [ ] **Analytics**: Usage metrics and reporting

---

## 🤝 **Contributing**

We welcome contributions! This project is specifically focused on AI agent workflows, so please keep that use case in mind.

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

---

## 📄 **License**

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## 🆘 **Support**

- 📖 **Documentation**: Check the [docs/](docs/) directory
- 🐛 **Issues**: [GitHub Issues](https://github.com/nikpietanze/linear-cli/issues)
- 💬 **Discussions**: [GitHub Discussions](https://github.com/nikpietanze/linear-cli/discussions)

---

**Made with ❤️ for AI agents and automation workflows**