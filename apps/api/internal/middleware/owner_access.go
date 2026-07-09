package middleware

import (
	"net/http"

	"lapangango-api/internal/httputil"
	"lapangango-api/internal/owneraccess"

	"github.com/gin-gonic/gin"
)

func OwnerWorkspaceAccess(repo *owneraccess.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		actorUserID, exists := httputil.GetAuthenticatedUserID(c)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
			return
		}

		role, _ := c.Get("auth_role")
		roleStr, _ := role.(string)

		if roleStr == "OWNER" {
			info, err := repo.GetOwnerContextByUserID(c.Request.Context(), actorUserID)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "Owner profile not found"})
				return
			}

			if info.OwnerUserStatus != "ACTIVE" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "Owner user account is not active"})
				return
			}

			c.Set("auth_actor_user_id", actorUserID)
			c.Set("auth_effective_owner_user_id", info.OwnerUserID)
			c.Set("auth_owner_profile_id", info.OwnerProfileID)
			c.Set("auth_owner_verification_status", info.VerificationStatus)
			c.Set("auth_is_owner", true)
			c.Next()
			return
		}

		if roleStr == "STAFF" {
			info, err := repo.GetStaffContextByUserID(c.Request.Context(), actorUserID)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "Forbidden"})
				return
			}

			if info.StaffUserStatus != "ACTIVE" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "Staff user account is not active"})
				return
			}

			if info.StaffStatus != "ACTIVE" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "Staff membership is not active"})
				return
			}

			if info.OwnerStatus != "ACTIVE" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "Owner user account is not active"})
				return
			}

			c.Set("auth_actor_user_id", actorUserID)
			c.Set("auth_effective_owner_user_id", info.OwnerUserID)
			c.Set("auth_owner_profile_id", info.OwnerProfileID)
			c.Set("auth_staff_member_id", info.StaffMemberID)
			c.Set("auth_staff_permissions", info.Permissions)
			c.Set("auth_staff_venue_ids", info.VenueIDs)
			c.Set("auth_is_owner", false)
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "Forbidden"})
	}
}

func RequireOwnerPermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if httputil.IsWorkspaceOwner(c) {
			c.Next()
			return
		}

		permissions := httputil.GetStaffPermissions(c)
		hasPermission := false
		for _, p := range permissions {
			if p == permission {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "You do not have permission to access this resource"})
			return
		}

		c.Next()
	}
}

func RequireActualOwner() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !httputil.IsWorkspaceOwner(c) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "You do not have permission to access this resource"})
			return
		}
		c.Next()
	}
}
