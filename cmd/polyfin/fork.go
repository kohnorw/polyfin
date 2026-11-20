package main

// This fork's own tasks (kohnorw/polyfin), kept apart from upstream's.

import (
	"context"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/tasks"
)

// registerForkTasks registers the fork's scheduled tasks: the daily check
// of the IPTV logins users registered with.
func registerForkTasks(registry *tasks.Registry, store *accounts.Store, channels *iptv.Service) {
	registry.Register(tasks.Task{
		Key:      "CheckXtreamLogins",
		Category: tasks.CategoryMaintenance,
		Text: map[string]tasks.Text{
			"en":    {Name: "Check IPTV logins", Description: "Turns off Live TV for users whose IPTV login no longer works."},
			"fr":    {Name: "Vérifier les identifiants IPTV", Description: "Désactive la TV en direct des utilisateurs dont l'identifiant IPTV ne fonctionne plus."},
			"de":    {Name: "IPTV-Anmeldungen prüfen", Description: "Schaltet Live-TV für Benutzer ab, deren IPTV-Anmeldung nicht mehr funktioniert."},
			"es":    {Name: "Comprobar los accesos IPTV", Description: "Desactiva la TV en directo de los usuarios cuyo acceso IPTV ya no funciona."},
			"it":    {Name: "Verifica gli accessi IPTV", Description: "Disattiva la TV in diretta per gli utenti il cui accesso IPTV non funziona più."},
			"pt-BR": {Name: "Verificar logins de IPTV", Description: "Desativa a TV ao vivo dos usuários cujo login de IPTV não funciona mais."},
		},
		Triggers: []tasks.Trigger{tasks.Every(24 * time.Hour)},
		Run:      func(ctx context.Context) error { return store.CheckXtreamLogins(ctx, channels.VerifyXtreamLogin) },
	})
}
