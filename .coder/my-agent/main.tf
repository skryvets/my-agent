terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    docker = {
      source = "kreuzwerker/docker"
    }
  }
}

variable "docker_socket" {
  default     = ""
  description = "(Optional) Docker socket URI"
  type        = string
}

provider "docker" {
  host = var.docker_socket != "" ? var.docker_socket : null
}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

data "coder_external_auth" "github" {
  id = "github"
}

data "coder_parameter" "repo_url" {
  name         = "repo_url"
  display_name = "Repository"
  description  = "The my-agent repository to clone into the workspace."
  type         = "string"
  default      = "https://github.com/skryvets/my-agent.git"
  mutable      = false
  order        = 1
}

data "coder_parameter" "go_version" {
  name         = "go_version"
  display_name = "Go version"
  description  = "Go toolchain to install. Must satisfy the `go` directive in go.mod."
  type         = "string"
  default      = "1.26.5"
  mutable      = true
  order        = 2
  validation {
    regex = "^[0-9]+\\.[0-9]+(\\.[0-9]+)?$"
    error = "Use a release number such as 1.26.5."
  }
}

data "coder_parameter" "model" {
  name         = "model"
  display_name = "MY_AGENT_MODEL"
  description  = "OpenRouter model slug the agent talks to."
  type         = "string"
  default      = "anthropic/claude-sonnet-4.5"
  mutable      = true
  order        = 3
}

locals {
  project_dir = "/home/coder/my-agent"
  secrets_env = "/home/coder/.config/my-agent/env"
}

resource "coder_agent" "main" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"

  env = {
    GIT_AUTHOR_NAME     = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_AUTHOR_EMAIL    = data.coder_workspace_owner.me.email
    GIT_COMMITTER_NAME  = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_COMMITTER_EMAIL = data.coder_workspace_owner.me.email
  }

  startup_script_behavior = "non-blocking"
  startup_script          = <<-EOT
    set -eu

    if [ ! -f ~/.init_done ]; then
      cp -rT /etc/skel ~ 2>/dev/null || true
      touch ~/.init_done
    fi

    # The Go toolchain lives in the persistent home volume, so a restart does
    # not re-download it, and a version parameter change does.
    want="go${data.coder_parameter.go_version.value}"
    if [ "$(~/.local/go/bin/go env GOVERSION 2>/dev/null || true)" != "$want" ]; then
      echo "installing $want"
      mkdir -p ~/.local
      rm -rf ~/.local/go
      curl -fsSL "https://go.dev/dl/$want.linux-${data.coder_provisioner.me.arch}.tar.gz" | tar -C ~/.local -xz
    fi

    mkdir -p ~/.config/my-agent
    if [ ! -f "${local.secrets_env}" ]; then
      cat > "${local.secrets_env}" <<'SECRETS'
    # Fill these in, then re-open your shell. This file is never sent to Coder.
    export OPENROUTER_API_KEY=
    export TELEGRAM_BOT_TOKEN=
    export TELEGRAM_ALLOWED_USERS=
    SECRETS
      chmod 600 "${local.secrets_env}"
    fi

    if ! grep -q 'my-agent workspace' ~/.bashrc 2>/dev/null; then
      cat >> ~/.bashrc <<'PROFILE'

    # my-agent workspace
    export PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH"
    [ -f "$HOME/.config/my-agent/env" ] && . "$HOME/.config/my-agent/env"
    PROFILE
    fi

    if [ ! -d "${local.project_dir}/.git" ]; then
      # The repository is private. Coder's askpass helper hands git the token
      # from the external auth above; GIT_TERMINAL_PROMPT=0 turns a missing
      # token into a failure instead of a startup script that hangs forever.
      GIT_TERMINAL_PROMPT=0 git clone "${data.coder_parameter.repo_url.value}" "${local.project_dir}"
    fi

    cd "${local.project_dir}"
    ~/.local/go/bin/go mod download
    ~/.local/go/bin/go build ./...
  EOT

  metadata {
    display_name = "CPU Usage"
    key          = "0_cpu_usage"
    script       = "coder stat cpu"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "RAM Usage"
    key          = "1_ram_usage"
    script       = "coder stat mem"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Home Disk"
    key          = "2_home_disk"
    script       = "coder stat disk --path $${HOME}"
    interval     = 60
    timeout      = 1
  }

  metadata {
    display_name = "Go version"
    key          = "3_go_version"
    script       = "$${HOME}/.local/go/bin/go env GOVERSION"
    interval     = 600
    timeout      = 5
  }
}

resource "coder_env" "model" {
  agent_id = coder_agent.main.id
  name     = "MY_AGENT_MODEL"
  value    = data.coder_parameter.model.value
}

module "code-server" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/code-server/coder"
  version  = "~> 1.0"
  agent_id = coder_agent.main.id
  folder   = local.project_dir
  order    = 1
}

resource "coder_app" "chat" {
  agent_id     = coder_agent.main.id
  slug         = "chat"
  display_name = "Terminal chat"
  icon         = "/icon/terminal.svg"
  command      = "cd ${local.project_dir} && go run ."
  order        = 2
}

resource "coder_app" "tests" {
  agent_id     = coder_agent.main.id
  slug         = "tests"
  display_name = "go test -race"
  icon         = "/icon/go.svg"
  command      = "cd ${local.project_dir} && go test ./... -race; exec bash"
  order        = 3
}

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"
  lifecycle {
    ignore_changes = all
  }
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.workspace_name_at_creation"
    value = data.coder_workspace.me.name
  }
}

resource "docker_container" "workspace" {
  count      = data.coder_workspace.me.start_count
  image      = "codercom/example-base:ubuntu"
  name       = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  hostname   = data.coder_workspace.me.name
  entrypoint = ["sh", "-c", replace(coder_agent.main.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")]
  env        = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  volumes {
    container_path = "/home/coder"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }

  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.workspace_name"
    value = data.coder_workspace.me.name
  }
}
