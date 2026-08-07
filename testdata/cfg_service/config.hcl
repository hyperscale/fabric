provider "service" {
  name             = "probe"
  version          = "1.2.3"
  shutdown_timeout = "45s"
  pre_stop_delay   = "3s"
}
