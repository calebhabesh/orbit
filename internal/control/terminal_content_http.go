package control

import (
	"io"
	"net/http"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
)

func (s *Server) registerTerminalContent(mux *http.ServeMux) {
	mux.HandleFunc("POST /control/terminal/v1/read", func(w http.ResponseWriter, r *http.Request) {
		var intent tc.ReadIntent
		if len(r.Header.Get("X-Orbit-Intent")) > 8192 || tc.Decode([]byte(r.Header.Get("X-Orbit-Intent")), &intent) != nil {
			writeError(w, terminalError("INVALID_REQUEST"))
			return
		}
		read, err := s.ctrl.ExactRead(r.Context(), intent)
		if err != nil {
			writeError(w, err)
			return
		}
		defer read.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		if _, err = io.CopyBuffer(w, read, make([]byte, 64<<10)); err != nil {
			panic(http.ErrAbortHandler)
		}
	})
	mux.HandleFunc("POST /control/terminal/v1/upload", func(w http.ResponseWriter, r *http.Request) {
		var intent tc.UploadIntent
		if len(r.Header.Get("X-Orbit-Intent")) > 8192 || tc.Decode([]byte(r.Header.Get("X-Orbit-Intent")), &intent) != nil {
			writeError(w, terminalError("INVALID_REQUEST"))
			return
		}
		result, err := s.ctrl.TerminalUpload(r.Context(), intent, r.Body)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}
