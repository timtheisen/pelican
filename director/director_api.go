/***************************************************************
 *
 * Copyright (C) 2023, Pelican Project, Morgridge Institute for Research
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you
 * may not use this file except in compliance with the License.  You may
 * obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 ***************************************************************/

package director

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/pelicanplatform/pelican/config"
	"github.com/pelicanplatform/pelican/param"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// List all namespaces from origins registered at the director
func ListNamespacesFromOrigins() []NamespaceAd {

	serverAdMutex.RLock()
	defer serverAdMutex.RUnlock()

	serverAdItems := serverAds.Items()
	namespaces := make([]NamespaceAd, 0, len(serverAdItems))
	for _, item := range serverAdItems {
		if item.Key().Type == OriginType {
			namespaces = append(namespaces, item.Value()...)
		}
	}
	return namespaces
}

func LoadDirectorPublicKey() (*jwk.Key, error) {
	directorDiscoveryUrlStr := param.Federation_DirectorUrl.GetString()
	if len(directorDiscoveryUrlStr) == 0 {
		return nil, errors.Errorf("Director URL is unset; Can't load director's public key")
	}
	log.Debugln("Director's discovery URL:", directorDiscoveryUrlStr)
	directorDiscoveryUrl, err := url.Parse(directorDiscoveryUrlStr)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintln("Invalid director URL:", directorDiscoveryUrlStr))
	}
	directorDiscoveryUrl.Scheme = "https"
	directorDiscoveryUrl.Path = directorDiscoveryUrl.Path + "/.well-known/pelican-configuration"

	tr := config.GetTransport()
	client := &http.Client{Transport: tr}

	req, err := http.NewRequest(http.MethodGet, directorDiscoveryUrl.String(), nil)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintln("Failure when doing director metadata request creation for: ", directorDiscoveryUrl))
	}

	result, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintln("Failure when doing director metadata lookup to: ", directorDiscoveryUrl))
	}

	if result.Body != nil {
		defer result.Body.Close()
	}

	body, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintln("Failure when doing director metadata read to: ", directorDiscoveryUrl))
	}

	metadata := DiscoveryResponse{}

	err = json.Unmarshal(body, &metadata)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintln("Failure when parsing director metadata at: ", directorDiscoveryUrl))
	}

	jwksUri := metadata.JwksUri

	response, err := client.Get(jwksUri)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintln("Failure when requesting director Jwks URI: ", jwksUri))
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintln("Failure when requesting director Jwks URI: ", jwksUri))
	}
	keys, err := jwk.Parse(contents)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintln("Failure when parsing director's jwks: ", jwksUri))
	}
	key, ok := keys.Key(0)
	if !ok {
		return nil, errors.Wrap(err, fmt.Sprintln("Failure when getting director's first public key: ", jwksUri))
	}

	return &key, nil
}

// Create a token for director's Prometheus instance to access
// director's origins service discovery endpoint. This function is intended
// to be called on a director server
func CreateDirectorSDToken() (string, error) {
	directorURL := param.Federation_DirectorUrl.GetString()
	if directorURL == "" {
		return "", errors.New("Director URL is not known; cannot create director service discovery token")
	}

	tok, err := jwt.NewBuilder().
		Claim("scope", "pelican.directorSD").
		Issuer(directorURL).
		Audience([]string{directorURL}).
		Subject("director").
		Expiration(time.Now().Add(time.Hour)).
		Build()
	if err != nil {
		return "", err
	}

	key, err := config.GetOriginJWK()
	if err != nil {
		return "", errors.Wrap(err, "failed to load the director's JWK")
	}

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.ES256, key))
	if err != nil {
		return "", err
	}
	return string(signed), nil
}

// Verify that a token received is a valid token from director and has
// correct scope for accessing the service discovery endpoint. This function
// is intended to be called on the same director server that issues the token.
func VerifyDirectorSDToken(strToken string) (bool, error) {
	directorURL := param.Federation_DirectorUrl.GetString()
	token, err := jwt.Parse([]byte(strToken), jwt.WithVerify(false))
	if err != nil {
		return false, err
	}

	if directorURL != token.Issuer() {
		return false, errors.Errorf("Token issuer is not a director")
	}
	// Given that this function is intended to be called on the same director server
	// that issues the token. so it's safe to skip getting the public key
	// from director's discovery URL.
	issuerKeyfile := param.IssuerKey.GetString()
	key, err := config.LoadPublicKey("", issuerKeyfile)
	if err != nil {
		return false, err
	}
	tok, err := jwt.Parse([]byte(strToken), jwt.WithKeySet(key), jwt.WithValidate(true))
	if err != nil {
		return false, err
	}

	scope_any, present := tok.Get("scope")
	if !present {
		return false, errors.New("No scope is present; required to advertise to director")
	}
	scope, ok := scope_any.(string)
	if !ok {
		return false, errors.New("scope claim in token is not string-valued")
	}

	scopes := strings.Split(scope, " ")

	for _, scope := range scopes {
		if scope == "pelican.directorSD" {
			return true, nil
		}
	}
	return false, nil
}
