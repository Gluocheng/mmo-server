package persistence

import (
	"context"
	"fmt"
	"sync"
	"time"

	cprofile "github.com/cherry-game/cherry/profile"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type config struct {
	mysqlDSN    string
	maxOpenConn int
	maxIdleConn int
	cacheTTL    time.Duration
	accessTTL   time.Duration
	refreshTTL  time.Duration
	loginFailLimit int
	loginFailWindow time.Duration
	loginBlockTTL time.Duration
	keyPrefix   string
}

var (
	once    sync.Once
	initErr error
	db      *gorm.DB
	rdb     *redis.Client
	cfg     config
)

func Init() error {
	once.Do(func() {
		cfg = loadConfig()
		if cfg.mysqlDSN == "" {
			initErr = fmt.Errorf("mysql.dsn is empty in profile")
			return
		}

		gdb, err := gorm.Open(mysql.Open(cfg.mysqlDSN), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			initErr = err
			return
		}
		sqlDB, err := gdb.DB()
		if err != nil {
			initErr = err
			return
		}
		sqlDB.SetMaxOpenConns(cfg.maxOpenConn)
		sqlDB.SetMaxIdleConns(cfg.maxIdleConn)
		sqlDB.SetConnMaxLifetime(time.Hour)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		rcfg := cprofile.GetConfig("redis")
		rdb = redis.NewClient(&redis.Options{
			Addr:         rcfg.GetString("address", "127.0.0.1:6379"),
			Password:     rcfg.GetString("password", ""),
			DB:           rcfg.GetInt("db", 0),
			PoolSize:     rcfg.GetInt("pool_size", 16),
			DialTimeout:  time.Duration(rcfg.GetInt("dial_timeout", 3)) * time.Second,
			ReadTimeout:  time.Duration(rcfg.GetInt("read_timeout", 2)) * time.Second,
			WriteTimeout: time.Duration(rcfg.GetInt("write_timeout", 2)) * time.Second,
		})
		if err = rdb.Ping(ctx).Err(); err != nil {
			initErr = err
			return
		}

		if err = gdb.WithContext(ctx).AutoMigrate(&Account{}, &Player{}); err != nil {
			initErr = err
			return
		}
		db = gdb
	})
	return initErr
}

func loadConfig() config {
	my := cprofile.GetConfig("mysql")
	rd := cprofile.GetConfig("redis")
	return config{
		mysqlDSN:    my.GetString("dsn", ""),
		maxOpenConn: my.GetInt("max_open_conns", 20),
		maxIdleConn: my.GetInt("max_idle_conns", 10),
		cacheTTL:    time.Duration(rd.GetInt("cache_ttl_seconds", 600)) * time.Second,
		accessTTL:   time.Duration(rd.GetInt("access_ttl_seconds", 900)) * time.Second,
		refreshTTL:  time.Duration(rd.GetInt("refresh_ttl_seconds", rd.GetInt("token_ttl_seconds", 86400))) * time.Second,
		loginFailLimit: rd.GetInt("login_fail_limit", 5),
		loginFailWindow: time.Duration(rd.GetInt("login_fail_window_seconds", 300)) * time.Second,
		loginBlockTTL: time.Duration(rd.GetInt("login_block_seconds", 600)) * time.Second,
		keyPrefix:   rd.GetString("key_prefix", "mmo"),
	}
}

func CacheTTL() time.Duration {
	return cfg.cacheTTL
}

func KeyPrefix() string {
	return cfg.keyPrefix
}

func AccessTTL() time.Duration {
	return cfg.accessTTL
}

func RefreshTTL() time.Duration {
	return cfg.refreshTTL
}

func LoginFailLimit() int {
	return cfg.loginFailLimit
}

func LoginFailWindow() time.Duration {
	return cfg.loginFailWindow
}

func LoginBlockTTL() time.Duration {
	return cfg.loginBlockTTL
}
