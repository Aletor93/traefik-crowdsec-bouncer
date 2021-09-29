package controler

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"

	. "github.com/fbonalair/traefik-crowdsec-bouncer/config"
	"github.com/fbonalair/traefik-crowdsec-bouncer/model"
	"github.com/gin-gonic/gin"
)

const ClientIpHeader = "X-Real-Ip"
const crowdsecAuthHeader = "X-Api-Key"

var crowdsecBouncerApiKey = RequiredEnv("CROWDSEC_BOUNCER_API_KEY")
var crowdsecBouncerHost = RequiredEnv("CROWDSEC_BOUNCER_HOST")
var crowdsecBouncerScheme = OptionalEnv("CROWDSEC_BOUNCER_SCHEME", "http")

func Ping(c *gin.Context) {
	c.String(http.StatusOK, "pong")
}

func ForwardAuth(c *gin.Context) {
	// Getting and verifying ip from header
	realIP := c.Request.Header.Get(ClientIpHeader)
	parsedRealIP := net.ParseIP(realIP)
	if parsedRealIP == nil {
		remedyError(fmt.Errorf("the header %q isn't a valid IP adress", ClientIpHeader), c)
		return
	}

	// Call crowdsec API
	tr := &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 30 * time.Second,
	}
	client := &http.Client{Transport: tr}
	decisionUrl := url.URL{
		Scheme:   crowdsecBouncerScheme,
		Host:     crowdsecBouncerHost,
		Path:     "v1/decisions",
		RawQuery: fmt.Sprintf("type=ban&ip=%s", realIP),
	}

	req, err := http.NewRequest(http.MethodGet, decisionUrl.String(), nil)
	if err != nil {
		remedyError(fmt.Errorf("can't create a new http request : %w", err), c)
		return
	}
	req.Header.Add(crowdsecAuthHeader, crowdsecBouncerApiKey)
	resp, err := client.Do(req)
	if err != nil {
		remedyError(fmt.Errorf("error while requesting crowdsec API : %w", err), c)
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			remedyError(err, c)
		}
	}(resp.Body)
	reqBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		remedyError(fmt.Errorf("error while parsing crowdsec response body : %w", err), c)
		return
	}

	var decisions []model.Decision
	err = json.Unmarshal(reqBody, &decisions)
	if err != nil {
		remedyError(fmt.Errorf("error while unmarshalling crowdsec response body : %w", err), c)
		return
	}

	// Authorization logic
	if len(decisions) > 0 {
		c.Status(http.StatusUnauthorized)
	} else {
		c.Status(http.StatusOK)
	}
}

func remedyError(err error, c *gin.Context) {
	_ = c.Error(err) // nil err should be handled earlier
	c.Status(http.StatusUnauthorized)
}