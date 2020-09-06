// +build linux

package server

/*
#include <shadow.h>
#include <stddef.h>
#include <stdlib.h>
*/
import "C"
import (
	"log"
	"strings"

	"github.com/tredoe/osutil/user/crypt"
	"github.com/tredoe/osutil/user/crypt/sha512_crypt"
)

/*
 * Authenticate gets the argument of an	authentication requests and returns
 * the user's hashed token if it authentication or else and empty string
 */
func Authenticate(args *AuthArgs) string {
	var hash string
	var c crypt.Crypter
	var err error

	sp := C.getspnam(C.CString(args.Username))
	if sp == nil {
		log.Printf("Failed to get user details >\nIf this happens for valid user, maybe we're not running as root")
		return ""
	}
	pwdp := C.GoString(sp.sp_pwdp)
	i := 0
	salt := strings.IndexFunc(pwdp, func(r rune) bool {
		if r == '$' {
			i++
		}
		return i == 3
	})
	s := []byte(pwdp)[:salt]
	t := string(pwdp)[salt:]
	if t == args.Secret {
		goto HappyEnd
	}
	c = sha512_crypt.New()
	hash, err = c.Generate([]byte(args.Secret), s)
	if err != nil {
		log.Printf("Got an error generate the hash. salt: %q", pwdp[:salt])
	}
	if string(hash) != pwdp {
		return ""
	}
HappyEnd:
	return t

}
