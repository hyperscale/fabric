variable "env_name" {
  value = "test"
}

provider "example" {
  name = var.env_name
}
