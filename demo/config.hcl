
provider "service" {
    # Bounds the whole drain: readiness off, pre-stop delay, waiting for the
    # runnable providers and every Stop share this single budget.
    shutdown_timeout = "30s"

    # Gap between turning readiness off and tearing anything down, so a load
    # balancer can observe the failing probe first.
    pre_stop_delay = "0s"
}

provider "logger" {
    level = "debug"
    format = "console"
}

provider "mysql" {
    host = "localhost"
    port = 3306
    username = "test"
    password = "Test!100"
    database = "test"
}
