# 🤖 AI Agent Examples

This document provides practical examples of how AI agents can use linear-cli for various automation scenarios.

## 🚀 Quick Start for AI Agents

### Basic Issue Creation
```bash
# Single command creates a fully structured issue
linear-cli issues create --team ENG --template "Feature Template" --title "Add user authentication" \
  --sections Summary="Implement secure user login system" \
  --sections Context="Users need secure access to protected features" \
  --sections Requirements="OAuth integration, password reset, 2FA support" \
  --sections "Definition of Done"="Authentication system is secure and user-friendly"
```

## 🔄 Automation Workflows

### GitHub Actions Integration

Create `.github/workflows/linear-issue.yml`:

```yaml
name: Create Linear Issue on Deployment Failure

on:
  workflow_run:
    workflows: ["Deploy"]
    types: [completed]
    branches: [main]

jobs:
  create-issue-on-failure:
    if: ${{ github.event.workflow_run.conclusion == 'failure' }}
    runs-on: ubuntu-latest
    steps:
      - name: Create Linear Issue
        run: |
          linear-cli issues create --team DEVOPS --template "Incident Template" \
            --title "Deployment Failed: ${{ github.event.workflow_run.head_sha }}" \
            --sections Summary="Production deployment failed and needs investigation" \
            --sections Context="Branch: ${{ github.event.workflow_run.head_branch }}, Commit: ${{ github.event.workflow_run.head_sha }}" \
            --sections "Steps to Reproduce"="1. Check deployment logs 2. Review failed commit 3. Identify root cause" \
            --sections Priority="High - Production impact"
        env:
          LINEAR_API_KEY: ${{ secrets.LINEAR_API_KEY }}
```

### Monitoring Integration

```bash
#!/bin/bash
# monitoring-alert.sh - Create Linear issues from monitoring alerts

ALERT_NAME="$1"
ALERT_SEVERITY="$2"
ALERT_DESCRIPTION="$3"

linear-cli issues create --team SRE --template "Incident Template" \
  --title "Alert: $ALERT_NAME" \
  --sections Summary="$ALERT_DESCRIPTION" \
  --sections Context="Monitoring system detected an issue requiring attention" \
  --sections Severity="$ALERT_SEVERITY" \
  --sections "Next Steps"="1. Investigate alert 2. Identify root cause 3. Implement fix"
```

## 🤖 AI Assistant Integration

### ChatGPT/Claude Integration

```python
import subprocess
import json

def create_linear_issue(team, template, title, sections):
    """Create a Linear issue from AI conversation context"""
    
    cmd = [
        "linear-cli", "issues", "create",
        "--team", team,
        "--template", template,
        "--title", title,
        "--json"  # Get structured response
    ]
    
    # Add sections
    for key, value in sections.items():
        cmd.extend(["--sections", f"{key}={value}"])
    
    result = subprocess.run(cmd, capture_output=True, text=True)
    
    if result.returncode == 0:
        issue_data = json.loads(result.stdout)
        return {
            "success": True,
            "issue_id": issue_data["identifier"],
            "url": issue_data["url"]
        }
    else:
        return {
            "success": False,
            "error": result.stderr
        }

# Example usage in AI assistant
def handle_user_request(user_message):
    if "create issue" in user_message.lower():
        # Extract requirements from conversation
        sections = {
            "Summary": "User requested feature from conversation",
            "Context": f"User said: {user_message}",
            "Requirements": "Extract specific requirements from user input"
        }
        
        result = create_linear_issue(
            team="ENG",
            template="Feature Template", 
            title="Feature request from user",
            sections=sections
        )
        
        if result["success"]:
            return f"Created issue {result['issue_id']}: {result['url']}"
        else:
            return f"Failed to create issue: {result['error']}"
```

### Slack Bot Integration

```javascript
// slack-bot.js - Create Linear issues from Slack messages
const { exec } = require('child_process');

app.command('/create-issue', async ({ command, ack, respond }) => {
  await ack();
  
  const { text } = command;
  const [title, ...descriptionParts] = text.split(' - ');
  const description = descriptionParts.join(' - ');
  
  const cmd = `linear-cli issues create --team ENG --template "Feature Template" \
    --title "${title}" \
    --sections Summary="${description}" \
    --sections Context="Created from Slack by ${command.user_name}" \
    --json`;
  
  exec(cmd, (error, stdout, stderr) => {
    if (error) {
      respond(`❌ Failed to create issue: ${stderr}`);
      return;
    }
    
    const issue = JSON.parse(stdout);
    respond(`✅ Created issue ${issue.identifier}: ${issue.url}`);
  });
});
```

## 📊 Batch Operations

### Create Multiple Issues from CSV

```bash
#!/bin/bash
# bulk-create-issues.sh - Create issues from CSV file

CSV_FILE="$1"
TEAM="$2"
TEMPLATE="$3"

# Skip header row and process each line
tail -n +2 "$CSV_FILE" | while IFS=',' read -r title summary context requirements; do
  echo "Creating issue: $title"
  
  linear-cli issues create --team "$TEAM" --template "$TEMPLATE" \
    --title "$title" \
    --sections Summary="$summary" \
    --sections Context="$context" \
    --sections Requirements="$requirements"
  
  # Rate limiting - wait 1 second between requests
  sleep 1
done
```

### Migration from Other Tools

```python
# migrate-from-jira.py - Migrate issues from JIRA to Linear
import csv
import subprocess
import time

def migrate_jira_issues(csv_file, team, template):
    with open(csv_file, 'r') as file:
        reader = csv.DictReader(file)
        
        for row in reader:
            sections = {
                "Summary": row["Summary"],
                "Context": f"Migrated from JIRA: {row['Key']}",
                "Requirements": row["Description"],
                "Definition of Done": row.get("Acceptance Criteria", "")
            }
            
            cmd = [
                "linear-cli", "issues", "create",
                "--team", team,
                "--template", template,
                "--title", row["Summary"]
            ]
            
            for key, value in sections.items():
                if value:  # Only add non-empty sections
                    cmd.extend(["--sections", f"{key}={value}"])
            
            result = subprocess.run(cmd, capture_output=True, text=True)
            
            if result.returncode == 0:
                print(f"✅ Migrated: {row['Key']} -> Linear issue created")
            else:
                print(f"❌ Failed to migrate {row['Key']}: {result.stderr}")
            
            # Rate limiting
            time.sleep(1)

# Usage
migrate_jira_issues("jira-export.csv", "ENG", "Feature Template")
```

## 🔍 Template Discovery

### Dynamic Template Usage

```bash
#!/bin/bash
# smart-issue-creation.sh - Automatically select template based on keywords

TITLE="$1"
DESCRIPTION="$2"
TEAM="$3"

# Get available templates
TEMPLATES=$(linear-cli issues template structure --team "$TEAM" --json | jq -r '.[]')

# Simple keyword-based template selection
if [[ "$TITLE" =~ [Bb]ug|[Ee]rror|[Ff]ix ]]; then
  TEMPLATE="Bug Template"
elif [[ "$TITLE" =~ [Ff]eature|[Aa]dd|[Ii]mplement ]]; then
  TEMPLATE="Feature Template"  
elif [[ "$TITLE" =~ [Rr]esearch|[Ss]pike|[Ii]nvestigate ]]; then
  TEMPLATE="Spike Template"
else
  TEMPLATE="Feature Template"  # Default
fi

echo "Selected template: $TEMPLATE"

linear-cli issues create --team "$TEAM" --template "$TEMPLATE" \
  --title "$TITLE" \
  --sections Summary="$DESCRIPTION"
```

## 📈 Analytics and Reporting

### Issue Creation Metrics

```bash
#!/bin/bash
# issue-metrics.sh - Track issue creation patterns

LOG_FILE="linear-cli-usage.log"

# Function to log issue creation
log_issue_creation() {
  local team="$1"
  local template="$2" 
  local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  
  echo "$timestamp,$team,$template,created" >> "$LOG_FILE"
}

# Create issue and log it
create_and_log() {
  local team="$1"
  local template="$2"
  local title="$3"
  shift 3
  
  if linear-cli issues create --team "$team" --template "$template" --title "$title" "$@"; then
    log_issue_creation "$team" "$template"
    echo "✅ Issue created and logged"
  else
    echo "❌ Issue creation failed"
  fi
}

# Usage
create_and_log "ENG" "Feature Template" "Add search functionality" \
  --sections Summary="Implement user search"
```

## 🛠️ Error Handling

### Robust Issue Creation

```python
import subprocess
import json
import time
import sys

def create_issue_with_retry(team, template, title, sections, max_retries=3):
    """Create Linear issue with retry logic and error handling"""
    
    for attempt in range(max_retries):
        try:
            cmd = [
                "linear-cli", "issues", "create",
                "--team", team,
                "--template", template, 
                "--title", title,
                "--json"
            ]
            
            for key, value in sections.items():
                cmd.extend(["--sections", f"{key}={value}"])
            
            result = subprocess.run(
                cmd, 
                capture_output=True, 
                text=True, 
                timeout=30
            )
            
            if result.returncode == 0:
                issue_data = json.loads(result.stdout)
                return {
                    "success": True,
                    "issue": issue_data,
                    "attempt": attempt + 1
                }
            else:
                error_msg = result.stderr.strip()
                
                # Handle specific errors
                if "template not found" in error_msg.lower():
                    # Try to sync templates and retry
                    sync_cmd = ["linear-cli", "templates", "sync", "--team", team]
                    subprocess.run(sync_cmd, capture_output=True)
                    continue
                elif "authentication" in error_msg.lower():
                    return {
                        "success": False,
                        "error": "Authentication failed. Check LINEAR_API_KEY.",
                        "retry": False
                    }
                else:
                    print(f"Attempt {attempt + 1} failed: {error_msg}")
                    if attempt < max_retries - 1:
                        time.sleep(2 ** attempt)  # Exponential backoff
                        continue
                    
        except subprocess.TimeoutExpired:
            print(f"Attempt {attempt + 1} timed out")
            if attempt < max_retries - 1:
                time.sleep(2 ** attempt)
                continue
        except Exception as e:
            print(f"Attempt {attempt + 1} failed with exception: {e}")
            if attempt < max_retries - 1:
                time.sleep(2 ** attempt)
                continue
    
    return {
        "success": False,
        "error": "All retry attempts failed",
        "retry": False
    }

# Example usage
sections = {
    "Summary": "Implement user authentication system",
    "Context": "Users need secure access to protected features",
    "Requirements": "OAuth, password reset, 2FA"
}

result = create_issue_with_retry("ENG", "Feature Template", "Add auth system", sections)

if result["success"]:
    print(f"✅ Issue created: {result['issue']['identifier']}")
else:
    print(f"❌ Failed to create issue: {result['error']}")
    sys.exit(1)
```

## 🔗 Integration Examples

### Webhook Handler

```python
# webhook-handler.py - Create Linear issues from webhook events
from flask import Flask, request, jsonify
import subprocess
import json

app = Flask(__name__)

@app.route('/webhook/create-issue', methods=['POST'])
def create_issue_webhook():
    data = request.json
    
    # Extract issue details from webhook payload
    title = data.get('title', 'Webhook Issue')
    description = data.get('description', '')
    team = data.get('team', 'ENG')
    template = data.get('template', 'Feature Template')
    
    # Create sections from webhook data
    sections = {
        "Summary": description,
        "Context": f"Created from webhook: {data.get('source', 'unknown')}",
        "Priority": data.get('priority', 'Medium')
    }
    
    # Create the issue
    cmd = [
        "linear-cli", "issues", "create",
        "--team", team,
        "--template", template,
        "--title", title,
        "--json"
    ]
    
    for key, value in sections.items():
        cmd.extend(["--sections", f"{key}={value}"])
    
    try:
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
        
        if result.returncode == 0:
            issue_data = json.loads(result.stdout)
            return jsonify({
                "success": True,
                "issue_id": issue_data["identifier"],
                "url": issue_data["url"]
            })
        else:
            return jsonify({
                "success": False,
                "error": result.stderr
            }), 400
            
    except Exception as e:
        return jsonify({
            "success": False,
            "error": str(e)
        }), 500

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000)
```

These examples demonstrate the flexibility and power of linear-cli for AI agents and automation workflows. The tool is designed to be simple, reliable, and perfect for programmatic issue management.
