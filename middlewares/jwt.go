package middlewares

import (
	"fmt"
	"github.com/danielgtaylor/huma/v2"
	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jwt"
	"strings"
)

// JWTValidationParameters is parameters for JWT token validation
type JWTValidationParameters struct {
	// ValidateIssuer - Validate Issuer in JWT token flag
	ValidateIssuer bool
	// ValidIssuer - Valid issuer JWT name, if empty ValidateIssuer flag is ignored
	ValidIssuer string
	// ValidateAudience - Validate Audience in JWT token flag
	ValidateAudience bool
	// ValidAudience - Valid JWT audience name, if empty ValidateAudience flag is ignored
	ValidAudience string
	// SignatureAlgorithm - Algorithm for JWT signature validation
	SignatureAlgorithm string
	// SignatureKey - JWT token Signature key for for JWT signature validation
	SignatureKey string
	// ValidateIssuerSigningKey - Validate signature flag, ignored if SignatureKey or SignatureAlgorithm is empty
	ValidateSignature bool
}

func (p *JWTValidationParameters) getValidatedParams() *JWTValidationParameters {
	correctParams := JWTValidationParameters{
		ValidateIssuer:     false,
		ValidIssuer:        p.ValidIssuer,
		ValidateAudience:   false,
		ValidAudience:      p.ValidAudience,
		SignatureAlgorithm: p.SignatureAlgorithm,
		SignatureKey:       p.SignatureKey,
		ValidateSignature:  false,
	}
	if len(correctParams.ValidIssuer) > 1 && p.ValidateIssuer {
		correctParams.ValidateIssuer = true
	}
	if len(correctParams.ValidAudience) > 1 && p.ValidateAudience {
		correctParams.ValidateAudience = true
	}
	if len(correctParams.SignatureKey) > 1 && len(p.SignatureAlgorithm) > 1 && p.ValidateSignature {
		correctParams.ValidateSignature = true
	}
	return &correctParams
}

type jwtMiddleware struct {
	params *JWTValidationParameters
}

func getBearerTokenFromContext(ctx huma.Context, in string) string {
	token := ""
	switch in {
	case "header":
		authHeader := ctx.Header("Authorization")
		if len(authHeader) > 7 && strings.ToLower(authHeader[0:6]) == "bearer" {
			token = authHeader[7:]
		}
	}
	return token
}

func parseJWTToken(token string, params *JWTValidationParameters) (jwt.Token, huma.StatusError) {
	var (
		jwtToken jwt.Token
		err      error
	)
	if params.ValidateSignature {
		jwtToken, err = jwt.Parse([]byte(token), jwt.WithVerify(jwa.SignatureAlgorithm(params.SignatureAlgorithm), []byte(params.SignatureKey)))
	} else {
		jwtToken, err = jwt.Parse([]byte(token))
	}
	if err != nil {
		return nil, huma.Error401Unauthorized(fmt.Sprintf("jwt: failed to parse token: %s", err.Error()))
	}
	return jwtToken, nil
}

func validateJWT(jwtToken jwt.Token, params *JWTValidationParameters) huma.StatusError {
	if jwtToken == nil {
		return huma.Error401Unauthorized("jwt: failed to validate token: token was not found")
	}
	var options []jwt.ValidateOption
	if params.ValidateIssuer {
		options = append(options, jwt.WithIssuer(params.ValidIssuer))
	}
	if params.ValidateAudience {
		options = append(options, jwt.WithAudience(params.ValidAudience))
	}
	err := jwt.Validate(jwtToken, options...)
	if err != nil {
		return huma.Error401Unauthorized(fmt.Sprintf("jwt: failed to validate token: %s", err.Error()))
	}
	return nil
}

func parseClaimSliceFromJWT[ValueType any](claims map[string]interface{}, claimKey string) []ValueType {
	slice := []ValueType{}
	claimsByType, ok := claims[claimKey]
	if !ok {
		return slice
	}
	typedInterfaceSliceClaims, ok := claimsByType.([]interface{})
	if !ok {
		elem, ok := claimsByType.(ValueType)
		if ok {
			slice = append(slice, elem)
		}
		return slice
	}
	slice = make([]ValueType, len(typedInterfaceSliceClaims))
	for i, interfaceClaimValue := range typedInterfaceSliceClaims {
		typedClaimValue, ok := interfaceClaimValue.(ValueType)
		if !ok {
			return slice
		}
		slice[i] = typedClaimValue
	}
	return slice
}

func sliceContainsElement[T comparable](s []T, elem T) bool {
	for _, a := range s {
		if a == elem {
			return true
		}
	}
	return false
}

func checkJWTAccess(jwtToken jwt.Token, anyOfRights []string) huma.StatusError {
	if jwtToken == nil {
		return huma.Error401Unauthorized("jwt: failed to check jwt claims: token was not found")
	}
	if len(anyOfRights) == 0 {
		return nil
	}
	mapAnyOfRights := map[string][]string{}
	for _, right := range anyOfRights {
		claim := strings.Split(right, ":")
		if len(claim) > 1 {
			mapAnyOfRights[claim[0]] = append(mapAnyOfRights[claim[0]], claim[1])
		}
	}
	for reqClaimType, reqAnyClaimValue := range mapAnyOfRights {
		values := parseClaimSliceFromJWT[string](jwtToken.PrivateClaims(), reqClaimType)
		for _, value := range values {
			if sliceContainsElement(reqAnyClaimValue, value) {
				return nil
			}
		}
	}
	return huma.Error403Forbidden(fmt.Sprintf("jwt: no rights, necessary rights: %s", strings.Join(anyOfRights, " or ")))
}

func (m *jwtMiddleware) handle(api huma.API, ctx huma.Context) huma.StatusError {
	for schemeName, _ := range api.OpenAPI().Components.SecuritySchemes {
		if strings.ToLower(schemeName) == "bearer" {
			var (
				authorizationRequired = false
				anyOfRights           []string
				ok                    bool
			)
			op := ctx.Operation()
			for _, opScheme := range op.Security {
				if anyOfRights, ok = opScheme[schemeName]; ok {
					authorizationRequired = true
				}
			}
			if !authorizationRequired {
				return nil
			}
			token := getBearerTokenFromContext(ctx, "header")
			if token == "" {
				return huma.Error401Unauthorized("jwt: token was not found")
			}
			jwtToken, err := parseJWTToken(token, m.params)
			if err != nil {
				return err
			}
			err = validateJWT(jwtToken, m.params)
			if err != nil {
				return err
			}
			err = checkJWTAccess(jwtToken, anyOfRights)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func addJWTAuthorizationMiddleware(params *JWTValidationParameters) func(huma.Handler) huma.Handler {
	return func(next huma.Handler) huma.Handler {
		hfn := func(api huma.API, ctx huma.Context) {
			err := (&jwtMiddleware{params}).handle(api, ctx)
			if err != nil {
				huma.WriteErr(api, ctx, err.GetStatus(), err.Error())
				return
			}
			next.Handle(api, ctx)
		}
		return huma.HandlerFunc(hfn)
	}
}

// NewHumaJwtMiddleware creates new jwt authorization middleware for huma.API
func NewHumaJwtMiddleware(params *JWTValidationParameters) func(huma.Handler) huma.Handler {
	return func(next huma.Handler) huma.Handler {
		return addJWTAuthorizationMiddleware(params.getValidatedParams())(next)
	}
}
