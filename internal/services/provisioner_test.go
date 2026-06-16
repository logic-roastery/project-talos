package services

import (
	"regexp"
	"strings"
	"testing"

	"github.com/logic-roastery/project-talos/internal/domain"
)

func TestGeneratePassword(t *testing.T) {
	t.Run("returns string of requested length", func(t *testing.T) {
		pw := GeneratePassword(24)
		if len(pw) != 24 {
			t.Errorf("expected length 24, got %d", len(pw))
		}
	})

	t.Run("returns empty string for length 0", func(t *testing.T) {
		pw := GeneratePassword(0)
		if pw != "" {
			t.Errorf("expected empty string, got %q", pw)
		}
	})

	t.Run("returns different values on successive calls", func(t *testing.T) {
		a := GeneratePassword(24)
		b := GeneratePassword(24)
		if a == b {
			t.Errorf("expected different passwords, both got %q", a)
		}
	})

	t.Run("uses URL-safe characters", func(t *testing.T) {
		pw := GeneratePassword(128)
		const allowed = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_.~"
		for _, ch := range pw {
			if !strings.ContainsRune(allowed, ch) {
				t.Fatalf("password contains non URL-safe character %q in %q", ch, pw)
			}
		}
	})
}

func TestGenerateAccessKey(t *testing.T) {
	t.Run("returns string of requested length", func(t *testing.T) {
		key := GenerateAccessKey(20)
		if len(key) != 20 {
			t.Errorf("expected length 20, got %d", len(key))
		}
	})

	t.Run("contains only uppercase ASCII and digits", func(t *testing.T) {
		key := GenerateAccessKey(20)
		re := regexp.MustCompile(`^[A-Z0-9]+$`)
		if !re.MatchString(key) {
			t.Errorf("key %q contains invalid characters", key)
		}
	})
}

func TestGenerateServiceName(t *testing.T) {
	tests := []struct {
		svcType  domain.ServiceType
		wantPref string
	}{
		{domain.ServicePostgres, "post"},
		{domain.ServiceRedis, "redi"},
		{domain.ServiceMySQL, "mysq"},
	}

	for _, tt := range tests {
		t.Run(string(tt.svcType), func(t *testing.T) {
			name := GenerateServiceName(tt.svcType)
			if len(name) != 9 {
				t.Errorf("expected length 9, got %d (%q)", len(name), name)
			}
			if name[:4] != tt.wantPref {
				t.Errorf("expected prefix %q, got %q", tt.wantPref, name[:4])
			}
			if name[4] != '-' {
				t.Errorf("expected '-' at position 4, got %q", name[4])
			}
		})
	}
}

func TestDefaultCredentials(t *testing.T) {
	t.Run("postgres", func(t *testing.T) {
		creds := DefaultCredentials(domain.ServicePostgres, "mycontainer")
		pc, ok := creds.(*domain.PostgresCredentials)
		if !ok {
			t.Fatalf("expected *domain.PostgresCredentials, got %T", creds)
		}
		if pc.Host != "mycontainer" {
			t.Errorf("Host: expected %q, got %q", "mycontainer", pc.Host)
		}
		if pc.Port != 5432 {
			t.Errorf("Port: expected 5432, got %d", pc.Port)
		}
		if pc.Database != "app" {
			t.Errorf("Database: expected %q, got %q", "app", pc.Database)
		}
		if pc.User != "postgres" {
			t.Errorf("User: expected %q, got %q", "postgres", pc.User)
		}
		if len(pc.Password) != 24 {
			t.Errorf("Password length: expected 24, got %d", len(pc.Password))
		}
	})

	t.Run("mysql", func(t *testing.T) {
		creds := DefaultCredentials(domain.ServiceMySQL, "mycontainer")
		mc, ok := creds.(*domain.MySQLCredentials)
		if !ok {
			t.Fatalf("expected *domain.MySQLCredentials, got %T", creds)
		}
		if mc.Host != "mycontainer" {
			t.Errorf("Host: expected %q, got %q", "mycontainer", mc.Host)
		}
		if mc.Port != 3306 {
			t.Errorf("Port: expected 3306, got %d", mc.Port)
		}
		if mc.Database != "app" {
			t.Errorf("Database: expected %q, got %q", "app", mc.Database)
		}
		if mc.User != "mysql" {
			t.Errorf("User: expected %q, got %q", "mysql", mc.User)
		}
		if len(mc.Password) != 24 {
			t.Errorf("Password length: expected 24, got %d", len(mc.Password))
		}
	})

	t.Run("redis", func(t *testing.T) {
		creds := DefaultCredentials(domain.ServiceRedis, "mycontainer")
		rc, ok := creds.(*domain.RedisCredentials)
		if !ok {
			t.Fatalf("expected *domain.RedisCredentials, got %T", creds)
		}
		if rc.Host != "mycontainer" {
			t.Errorf("Host: expected %q, got %q", "mycontainer", rc.Host)
		}
		if rc.Port != 6379 {
			t.Errorf("Port: expected 6379, got %d", rc.Port)
		}
	})

	t.Run("garage", func(t *testing.T) {
		creds := DefaultCredentials(domain.ServiceGarage, "mycontainer")
		gc, ok := creds.(*domain.GarageCredentials)
		if !ok {
			t.Fatalf("expected *domain.GarageCredentials, got %T", creds)
		}
		if !regexp.MustCompile(`mycontainer`).MatchString(gc.Endpoint) {
			t.Errorf("Endpoint: expected to contain %q, got %q", "mycontainer", gc.Endpoint)
		}
		if gc.Region != "garage" {
			t.Errorf("Region: expected %q, got %q", "garage", gc.Region)
		}
		if len(gc.AccessKey) != 20 {
			t.Errorf("AccessKey length: expected 20, got %d", len(gc.AccessKey))
		}
	})

	t.Run("unknown returns nil", func(t *testing.T) {
		creds := DefaultCredentials("unknown", "x")
		if creds != nil {
			t.Errorf("expected nil, got %v", creds)
		}
	})
}

func TestFormatEnvVars(t *testing.T) {
	t.Run("postgres", func(t *testing.T) {
		svc := &domain.Service{Type: domain.ServicePostgres}
		creds := &domain.PostgresCredentials{
			Host:     "pg-host",
			Port:     5432,
			Database: "app",
			User:     "postgres",
			Password: "s#cr@t:123",
		}
		vars := FormatEnvVars(svc, creds, "DB")
		joined := strings.Join(vars, "\n")

		for _, want := range []string{
			"DB_URL=postgres://postgres:s%23cr%40t%3A123@pg-host:5432/app",
			"DB_HOST=",
			"DB_PORT=",
			"DB_USER=",
			"DB_PASSWORD=s#cr@t:123",
			"DB_NAME=",
		} {
			if !strings.Contains(joined, want) {
				t.Errorf("expected output to contain %q", want)
			}
		}
	})

	t.Run("mysql", func(t *testing.T) {
		svc := &domain.Service{Type: domain.ServiceMySQL}
		creds := &domain.MySQLCredentials{
			Host:     "my-host",
			Port:     3306,
			Database: "app",
			User:     "mysql",
			Password: "s#cr@t:456",
		}
		vars := FormatEnvVars(svc, creds, "DB")
		joined := strings.Join(vars, "\n")

		for _, want := range []string{
			"DB_URL=mysql://mysql:s%23cr%40t%3A456@my-host:3306/app",
			"DB_HOST=",
			"DB_PORT=",
			"DB_USER=",
			"DB_PASSWORD=s#cr@t:456",
			"DB_NAME=",
		} {
			if !strings.Contains(joined, want) {
				t.Errorf("expected output to contain %q", want)
			}
		}
	})

	t.Run("redis", func(t *testing.T) {
		svc := &domain.Service{Type: domain.ServiceRedis}
		creds := &domain.RedisCredentials{
			Host:     "cache-host",
			Port:     6379,
			Password: "s#cr@t:789",
		}
		vars := FormatEnvVars(svc, creds, "CACHE")
		joined := strings.Join(vars, "\n")

		for _, want := range []string{
			"CACHE_URL=redis://:s%23cr%40t%3A789@cache-host:6379",
			"CACHE_HOST=",
			"CACHE_PORT=",
			"CACHE_PASSWORD=s#cr@t:789",
		} {
			if !strings.Contains(joined, want) {
				t.Errorf("expected output to contain %q", want)
			}
		}
	})

	t.Run("garage", func(t *testing.T) {
		svc := &domain.Service{Type: domain.ServiceGarage}
		creds := &domain.GarageCredentials{
			Endpoint:  "http://mycontainer:3900",
			Region:    "garage",
			AccessKey: "AKID1234567890123456",
			SecretKey: "secretkey1234567890123456789012345678",
			Bucket:    "my-bucket",
		}
		vars := FormatEnvVars(svc, creds, "S3")
		joined := strings.Join(vars, "\n")

		for _, want := range []string{
			"S3_ENDPOINT=",
			"S3_REGION=",
			"S3_ACCESS_KEY=",
			"S3_SECRET_KEY=",
			"S3_BUCKET=",
		} {
			if !strings.Contains(joined, want) {
				t.Errorf("expected output to contain %q", want)
			}
		}
	})
}
