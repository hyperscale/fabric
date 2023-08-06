
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
