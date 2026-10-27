package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/auth"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
)

var errMissingContext = errors.New("httpapi: request context is missing the identity or principal")

func (s *Server) requestLoginCode(w http.ResponseWriter, r *http.Request) {
	var body apigen.LoginCodeRequest
	if !s.decode(w, r, &body) {
		return
	}
	if err := s.Auth.RequestCode(r.Context(), body.Phone); err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusAccepted, apigen.StatusResponse{Status: "sent"})
}

func (s *Server) verifyLoginCode(w http.ResponseWriter, r *http.Request) {
	var body apigen.LoginCodeVerify
	if !s.decode(w, r, &body) {
		return
	}
	sess, id, err := s.Auth.VerifyCode(r.Context(), body.Phone, body.Code)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.setSessionCookie(w, sess)
	s.writeMe(w, r, id)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	id, ok := identityFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	if err := s.Auth.Logout(r.Context(), id.SessionID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getMe(w http.ResponseWriter, r *http.Request) {
	id, ok := identityFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	s.writeMe(w, r, id)
}

func (s *Server) writeMe(w http.ResponseWriter, r *http.Request, id auth.Identity) {
	orgs, err := s.Auth.Organizations(r.Context(), id.UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	resp := apigen.MeResponse{
		User:          apigen.User{Id: id.UserID, Name: id.Name, Phone: id.Phone},
		Organizations: make([]apigen.Organization, len(orgs)),
	}
	for i, o := range orgs {
		resp.Organizations[i] = apigen.Organization{Id: o.ID, Name: o.Name, DisplayName: o.DisplayName}
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) getOrgMe(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	resp := apigen.OrgMeResponse{
		MembershipId: p.MembershipID,
		Roles:        make([]apigen.RoleRef, len(p.Roles)),
	}
	for i, role := range p.Roles {
		resp.Roles[i] = apigen.RoleRef{Key: role.Key, Name: role.Name, Builtin: role.Builtin, PropertyId: optionalID(role.Property)}
	}
	for _, c := range authz.Capabilities(p) {
		resp.Capabilities = append(resp.Capabilities, apigen.Capability{
			Action: c.Action, Access: apigen.CapabilityAccess(c.Access),
		})
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	members, err := s.Members.List(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	resp := apigen.MemberList{Members: make([]apigen.Member, len(members))}
	for i, m := range members {
		resp.Members[i] = toAPIMember(m)
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) inviteMember(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	var body apigen.InviteMember
	if !s.decode(w, r, &body) {
		return
	}
	m, err := s.Members.Invite(r.Context(), p, member.Invite{Phone: body.Phone, Name: body.Name, Roles: roleGrants(body.Roles)})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, toAPIMember(m))
}

func (s *Server) setMemberRoles(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	memberID, err := uuid.Parse(chi.URLParam(r, "memberID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	var body apigen.SetRoles
	if !s.decode(w, r, &body) {
		return
	}
	m, err := s.Members.SetRoles(r.Context(), p, memberID, roleGrants(body.Roles))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPIMember(m))
}

// roleGrants converts the contract's role grants; a missing propertyId means the whole
// organization.
func roleGrants(in []apigen.RoleGrant) []member.RoleGrant {
	out := make([]member.RoleGrant, len(in))
	for i, g := range in {
		out[i] = member.RoleGrant{Role: string(g.Role)}
		if g.PropertyId != nil {
			out[i].PropertyID = *g.PropertyId
		}
	}
	return out
}

func toAPIMember(m member.Member) apigen.Member {
	out := apigen.Member{
		Id:        m.ID,
		UserId:    m.UserID,
		Name:      m.Name,
		Phone:     m.Phone,
		Status:    apigen.MemberStatus(m.Status),
		CreatedAt: m.CreatedAt,
		Roles:     make([]apigen.RoleRef, len(m.Roles)),
	}
	for i, role := range m.Roles {
		out.Roles[i] = apigen.RoleRef{Key: role.Key, Name: role.Name, Builtin: role.Builtin, PropertyId: optionalID(role.PropertyID)}
	}
	return out
}

// optionalID leaves an unset id out of the response.
func optionalID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
