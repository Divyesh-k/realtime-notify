// Command mint-token issues a JWT compatible with this service, for
// local testing without standing up a full auth service. NOT for
// production use -- the real auth service (e.g. the SaaS starter kit)
// is the only thing that should issue user-facing tokens there. This
// just wraps auth.Verifier.IssueDevToken, which exists in the auth
// package for exactly this purpose and is never wired into a route.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/yourname/realtime-notify/internal/auth"
)

func main() {
	secret := flag.String("secret", "dev-only-secret-do-not-use-in-production", "JWT signing secret, must match JWT_SECRET")
	userID := flag.String("user", "test-user-1", "user id (sub claim)")
	orgID := flag.String("org", "test-org-1", "org_id claim")
	ttl := flag.Duration("ttl", time.Hour, "token lifetime")
	flag.Parse()

	v := auth.NewVerifier(*secret)
	token, err := v.IssueDevToken(*userID, *orgID, *ttl)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error signing token:", err)
		os.Exit(1)
	}
	fmt.Println(token)
}
