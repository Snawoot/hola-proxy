package main

import (
    "time"
    "sync"
    "context"
)

const DEFAULT_LIST_LIMIT = 3
const API_CALL_ATTEMPTS = 3

func CredService(interval, timeout time.Duration,
                 country string,
                 proxytype string,
                 logger *CondLogger) (auth AuthProvider,
                                      tunnels *ZGetTunnelsResponse,
                                      err error) {
    var mux sync.Mutex
    var auth_header, user_uuid string
    auth = func () (res string) {
        (&mux).Lock()
        res = auth_header
        (&mux).Unlock()
        return
    }

    for i := 0; i < API_CALL_ATTEMPTS ; i++ {
        ctx, _ := context.WithTimeout(context.Background(), timeout)
        tunnels, user_uuid, err = Tunnels(ctx, country, proxytype, DEFAULT_LIST_LIMIT)
        if err == nil {
            break
        }
    }
    if err != nil {
        logger.Critical("Configuration bootstrap failed: %v", err)
        return
    }
    auth_header = basic_auth_header(LOGIN_PREFIX + user_uuid,
                                    tunnels.AgentKey)
    go func() {
        var (
            err error
            tuns *ZGetTunnelsResponse
            user_uuid string
        )
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        for {
            <-ticker.C
            logger.Info("Rotating credentials...")
            for i := 0; i < API_CALL_ATTEMPTS ; i++ {
                ctx, _ := context.WithTimeout(context.Background(), timeout)
                tuns, user_uuid, err = Tunnels(ctx, country, proxytype, DEFAULT_LIST_LIMIT)
                if err == nil {
                    break
                }
            }
            if err != nil {
                logger.Error("Credential rotation failed after %d attempts. Error: %v",
                             API_CALL_ATTEMPTS, err)
            } else {
                (&mux).Lock()
                auth_header = basic_auth_header(LOGIN_PREFIX + user_uuid,
                                                tuns.AgentKey)
                (&mux).Unlock()
                logger.Info("Credentials rotated successfully.")
            }
        }
    }()
    return
}
