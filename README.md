# Deal Archiver Service

A Golang-based service for automating the archival process of deals in vTiger CRM.
Processes deals in parallel and notifies administrators via email about operation results.

## Features

- **Scheduled Checks**: Periodically checks for deals needing archival
- **Parallel Processing**: Uses goroutines for concurrent deal processing
- **Status Management**: Automatically updates deal statuses through different stages
- **Email Notifications**: Sends success/error reports via SMTP
- **Customizable Configuration**: All parameters configurable via YAML file

## Prerequisites

- Go 1.24+
- MySQL/MariaDB database
- SMTP server credentials (Migadu supported out-of-box)
- Basic understanding of YAML configuration

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/yourusername/deal-archiver.git
   cd deal-archiver
    ```

2. Install dependencies:
   ```bash
   go get github.com/go-sql-driver/mysql
   go get gopkg.in/yaml.v3
   ```

3. Create configuration file:
   ```bash
   cp config.example.yml config.yml
    ```

## Configuration

Edit config.yml with your settings:

```yaml
production:
    host: "localhost"
    port: 3306
    user: "vtiger_user"
    password: "secure_password"
    name: "vtiger_db"

archive:
    check_interval: 60  # Seconds between checks
    source_status: "To Archive"
    archiving_status: "Archiving"
    archived_status: "Archived"
    error_status: "Error"
    archive_url: "https://dev.fundkite.com/vd_deals_archive.php"

smtp:
    server: "smtp.migadu.com"
    port: 465
    username: "user@yourdomain.com"
    password: "smtp_password"
    from: "noreply@yourdomain.com"
    to: "admin@yourdomain.com"
```

## Build and Run

```bash
go build -o deal-archiver
./deal-archiver
```

## Systemd Service (Optional)

Create /etc/systemd/system/deal-archiver.service:
```ini
[Unit]
Description=Deal Archiver Service
After=network.target

[Service]
User=dealuser
WorkingDirectory=/opt/deal-archiver
ExecStart=/opt/deal-archiver/deal-archiver
Restart=always

[Install]
WantedBy=multi-user.target
```

Enable and start:
```bash
sudo systemctl enable deal-archiver
sudo systemctl start deal-archiver
```

## Logging

By default, logs are written to stdout. For production, consider:

```bash
./deal-archiver >> /var/log/deal-archiver.log 2>&1
```

## Testing

Basic test:
```bash
curl "https://dev.fundkite.com/vd_deals_archive.php?deal=TEST_ID"
```

Check database:
```sql
SELECT * FROM vtiger_potential WHERE archive_status IN ('Archiving','Archived','Error');
```

## Security Considerations

- Store config.yml with proper permissions (chmod 600)
- Use environment variables for sensitive data (see [12factor config](https://12factor.net/config))
- Regularly rotate SMTP credentials
- Implement firewall rules for database access

## Troubleshooting

Common issues:
- **Connection errors**: Verify database/SMTP credentials
- **Status not updating**: Check MySQL user permissions
- **Email failures**: Test SMTP settings with telnet:

```bash
openssl s_client -connect smtp.migadu.com:465 -crlf
```


## License

MIT License - See [LICENSE](LICENSE) file

---

**Maintenance Note**: Requires periodic updates to maintain compatibility with vTiger API changes.