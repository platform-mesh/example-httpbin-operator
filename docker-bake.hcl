variable "REGISTRY" {
  default = "ghcr.io/platform-mesh"
}

variable "VERSION" {
  default = "dev"
}

variable "PLATFORMS" {
  default = "linux/amd64,linux/arm64"
}

variable "PLATFORMS_LIST" {
  default = split(",", PLATFORMS)
}

group "default" {
  targets = [
    "example-httpbin-operator",
  ]
}

target "example-httpbin-operator" {
  context = "."
  dockerfile = "Dockerfile"
  tags = [
    "${REGISTRY}/example-httpbin-operator:${VERSION}",
  ]
  platforms = PLATFORMS_LIST
}
