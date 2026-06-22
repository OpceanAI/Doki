package compose

import (
	"fmt"
	"strings"
)

// ValidationError represents a compose file validation error.
type ValidationError struct {
	Field   string
	Service string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Service != "" {
		return fmt.Sprintf("service %q: %s: %s", e.Service, e.Field, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors is a list of validation errors.
type ValidationErrors []*ValidationError

func (ve ValidationErrors) Error() string {
	msgs := make([]string, len(ve))
	for i, e := range ve {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}

// Validate validates a compose file.
func Validate(file *ComposeFile) error {
	if file == nil {
		return &ValidationError{Field: "file", Message: "compose file is nil"}
	}

	var errs ValidationErrors

	if len(file.Services) == 0 {
		errs = append(errs, &ValidationError{Field: "services", Message: "must have at least one service"})
	}

	// Validate each service.
	for name, svc := range file.Services {
		errs = append(errs, validateService(name, svc)...)
	}

	// Validate network references.
	for name, svc := range file.Services {
		if svc.NetworkMode != "" {
			if svc.NetworkMode != "host" && svc.NetworkMode != "none" && svc.NetworkMode != "bridge" {
				// Check if network is defined.
				if _, ok := file.Networks[svc.NetworkMode]; !ok {
					errs = append(errs, &ValidationError{
						Service: name,
						Field:   "network_mode",
						Message: fmt.Sprintf("network %q not defined", svc.NetworkMode),
					})
				}
			}
		}
	}

	// Validate volume references.
	for name, svc := range file.Services {
		vols := toStringSlice(svc.Volumes)
		for _, vol := range vols {
			parts := strings.SplitN(vol, ":", 3)
			if len(parts) >= 2 {
				src := parts[0]
				// Check if it's a named volume.
				if !strings.HasPrefix(src, "/") && !strings.HasPrefix(src, ".") && !strings.HasPrefix(src, "~") {
					if _, ok := file.Volumes[src]; !ok {
						// Could be a bind mount with relative path.
						if !strings.Contains(src, "/") && !strings.Contains(src, "\\") {
							errs = append(errs, &ValidationError{
								Service: name,
								Field:   "volumes",
								Message: fmt.Sprintf("volume %q not defined", src),
							})
						}
					}
				}
			}
		}
	}

	// Validate depends_on references.
	for name, svc := range file.Services {
		deps := parseDependsOn(svc.DependsOn)
		for dep := range deps {
			if _, ok := file.Services[dep]; !ok {
				errs = append(errs, &ValidationError{
					Service: name,
					Field:   "depends_on",
					Message: fmt.Sprintf("service %q not defined", dep),
				})
			}
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

func validateService(name string, svc *Service) ValidationErrors {
	var errs ValidationErrors

	// Must have image or build.
	if svc.Image == "" && svc.Build == nil {
		errs = append(errs, &ValidationError{
			Service: name,
			Field:   "image/build",
			Message: "must specify either image or build",
		})
	}

	// Validate restart policy.
	if svc.Restart != "" {
		validPolicies := map[string]bool{
			"no": true, "always": true, "on-failure": true, "unless-stopped": true,
		}
		if !validPolicies[svc.Restart] {
			errs = append(errs, &ValidationError{
				Service: name,
				Field:   "restart",
				Message: fmt.Sprintf("invalid restart policy %q (valid: no, always, on-failure, unless-stopped)", svc.Restart),
			})
		}
	}

	// Validate healthcheck.
	if svc.Healthcheck != nil {
		if svc.Healthcheck.Disable {
			// Disable is valid.
		} else {
			test := svc.Healthcheck.Test
			if test == nil {
				errs = append(errs, &ValidationError{
					Service: name,
					Field:   "healthcheck.test",
					Message: "test must be specified when healthcheck is not disabled",
				})
			}
		}
	}

	// Validate deploy resources.
	if svc.Deploy != nil && svc.Deploy.Resources != nil {
		if svc.Deploy.Resources.Limits != nil {
			if svc.Deploy.Resources.Limits.Memory != "" {
				if parseMemory(svc.Deploy.Resources.Limits.Memory) == 0 {
					errs = append(errs, &ValidationError{
						Service: name,
						Field:   "deploy.resources.limits.memory",
						Message: fmt.Sprintf("invalid memory value: %s", svc.Deploy.Resources.Limits.Memory),
					})
				}
			}
		}
	}

	// Validate scale.
	if svc.Scale < 0 {
		errs = append(errs, &ValidationError{
			Service: name,
			Field:   "scale",
			Message: "must be >= 0",
		})
	}

	// Validate pids_limit.
	if svc.PidsLimit < 0 {
		errs = append(errs, &ValidationError{
			Service: name,
			Field:   "pids_limit",
			Message: "must be >= 0",
		})
	}

	return errs
}

func parseDependsOn(deps interface{}) map[string]string {
	result := make(map[string]string)
	if deps == nil {
		return result
	}
	switch d := deps.(type) {
	case []interface{}:
		for _, dep := range d {
			switch v := dep.(type) {
			case string:
				result[v] = "service_started"
			case map[string]interface{}:
				name, _ := v["service"].(string)
				cond, _ := v["condition"].(string)
				if name != "" {
					if cond == "" {
						cond = "service_started"
					}
					result[name] = cond
				}
			}
		}
	case map[string]interface{}:
		for name, val := range d {
			if cond, ok := val.(string); ok {
				result[name] = cond
			} else {
				result[name] = "service_started"
			}
		}
	}
	return result
}
