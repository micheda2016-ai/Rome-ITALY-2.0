package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
)

func main() {
	// Server HTTP per mantenere Render sempre attivo e in salute sulla porta 10000
	port := os.Getenv("PORT")
	if port == "" {
		port = "10000"
	}
	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "Bot Rome Italy 2.0 Online!")
		})
		log.Printf("Server HTTP avviato sulla porta %s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Printf("Errore server HTTP: %v", err)
		}
	}()

	// Recupera e pulisce il token
	token := strings.TrimSpace(os.Getenv("DISCORD_TOKEN"))
	if token == "" {
		log.Println("⚠️ ERRORE: La variabile d'ambiente DISCORD_TOKEN non è impostata!")
	} else {
		if strings.HasPrefix(token, "Bot ") {
			token = strings.TrimPrefix(token, "Bot ")
		}

		dg, err := discordgo.New("Bot " + token)
		if err != nil {
			log.Printf("Errore durante la creazione della sessione Discord: %v", err)
		} else {
			dg.AddHandler(interactionHandler)

			err = dg.Open()
			if err != nil {
				log.Printf("Errore durante l'apertura della connessione a Discord: %v", err)
			} else {
				defer dg.Close()
				log.Println("✅ Bot 'Rome Italy 2.0' avviato con successo su Render!")
				registerCommands(dg)
			}
		}
	}

	// Mantiene il servizio attivo per il server HTTP
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	log.Println("Arresto del bot...")
}

func registerCommands(s *discordgo.Session) {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "setup",
			Description: "Configura i pannelli interattivi e i canali del server",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "server",
					Description: "Apre il pannello interattivo per configurare canali e ruoli",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "apri-conto",
					Description: "Invia il pannello bancario per l'apertura del conto",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "bonifico",
					Description: "Invia il pannello per l'invio dei bonifici",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "busta-paga",
					Description: "Invia il pannello per la riscossione dello stipendio",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "cittadinanza",
					Description: "Invia il pannello anagrafe per la carta d'identità",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "porto-darmi",
					Description: "Invia il pannello per la richiesta del porto d'armi",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "immatricolazione",
					Description: "Invia il pannello PRA per immatricolare i veicoli",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "registra-arresto",
					Description: "Invia il pannello delle Forze dell'Ordine per registrare arresti",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "emetti-multa",
					Description: "Invia il pannello per emettere sanzioni stradali",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "staff-panel",
					Description: "Invia il pannello riservato alla moderazione Staff",
				},
			},
		},
	}

	for _, cmd := range commands {
		_, err := s.ApplicationCommandCreate(s.State.User.ID, "", cmd)
		if err != nil {
			log.Printf("Errore nella registrazione del comando '%s': %v", cmd.Name, err)
		}
	}
}

func interactionHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		handleSlashCommands(s, i)
	case discordgo.InteractionMessageComponent:
		handleComponents(s, i)
	case discordgo.InteractionModalSubmit:
		handleModalSubmit(s, i)
	}
}

func handleSlashCommands(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		return
	}

	subCmd := options[0].Name

	switch subCmd {
	case "server":
		sendServerSetupPanel(s, i)
	case "apri-conto":
		sendApriContoPanel(s, i)
	case "busta-paga":
		sendBustaPagaPanel(s, i)
	case "cittadinanza":
		sendCittadinanzaPanel(s, i)
	case "registra-arresto":
		sendRegistraArrestoPanel(s, i)
	default:
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("✅ Pannello `/setup %s` inviato con successo!", subCmd),
			},
		})
	}
}

func sendServerSetupPanel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "⚙️ CONFIGURAZIONE GUIDATA - ROME ITALY 2.0",
		Description: "Usa il menu a tendina sottostante per scegliere quale sezione configurare. Potrai selezionare direttamente i canali e i ruoli del server!",
		Color:       0x0099FF,
	}

	selectMenu := discordgo.SelectMenu{
		CustomID:    "setup_server_select",
		Placeholder: "Scegli il servizio da configurare...",
		Options: []discordgo.SelectMenuOption{
			{Label: "Anagrafe & Cittadinanza", Value: "cfg_cittadinanza", Description: "Configura canali e ruoli cittadinanza", Emoji: &discordgo.ComponentEmoji{Name: "🇮🇹"}},
			{Label: "Banche & Conti Correnti", Value: "cfg_banche", Description: "Configura canali e ruoli banche", Emoji: &discordgo.ComponentEmoji{Name: "💳"}},
			{Label: "Forze dell'Ordine & Arresti", Value: "cfg_police", Description: "Configura canali verbali e ruoli FDO", Emoji: &discordgo.ComponentEmoji{Name: "🚨"}},
			{Label: "Busta Paga & Stipendi", Value: "cfg_stipendi", Description: "Configura accrediti e ruoli fazione", Emoji: &discordgo.ComponentEmoji{Name: "💵"}},
			{Label: "Logs Staff & Moderazione", Value: "cfg_staff", Description: "Configura canali log e ruoli staff", Emoji: &discordgo.ComponentEmoji{Name: "🛡️"}},
		},
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{selectMenu}},
			},
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
}

func sendApriContoPanel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "🏛️ APERTURA CONTO CORRENTE - ROME ITALY 2.0",
		Description: "Clicca sul bottone per scegliere il tuo istituto bancario.",
		Color:       0x2ECC71,
	}
	button := discordgo.Button{Label: "Apri Conto Corrente", Style: discordgo.SuccessButton, CustomID: "btn_apri_conto", Emoji: &discordgo.ComponentEmoji{Name: "💳"}}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{button}}}},
	})
}

func sendBustaPagaPanel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "💼 RISCOSSIONE BUSTA PAGA - ROME ITALY 2.0",
		Description: "Clicca sul bottone per riscattare lo stipendio.",
		Color:       0xF1C40F,
	}
	button := discordgo.Button{Label: "Riscuoti Busta Paga", Style: discordgo.PrimaryButton, CustomID: "btn_riscuoti_stipendio", Emoji: &discordgo.ComponentEmoji{Name: "💵"}}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{button}}}},
	})
}

func sendCittadinanzaPanel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "🇮🇹 ANAGRAFE CENTRALE - ROME ITALY 2.0",
		Description: "Richiedi la tua Carta d'Identità.",
		Color:       0x3498DB,
	}
	button := discordgo.Button{Label: "Richiedi Cittadinanza", Style: discordgo.PrimaryButton, CustomID: "btn_richiedi_cittadinanza", Emoji: &discordgo.ComponentEmoji{Name: "🆔"}}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{button}}}},
	})
}

func sendRegistraArrestoPanel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "🚨 BANCA DATI POLIZIA DI STATO / CARABINIERI",
		Description: "Verbale dei fermi e degli arresti.",
		Color:       0xE74C3C,
	}
	button := discordgo.Button{Label: "Registra Arresto", Style: discordgo.DangerButton, CustomID: "btn_registra_arresto", Emoji: &discordgo.ComponentEmoji{Name: "📝"}}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{button}}}},
	})
}

func handleComponents(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	if strings.HasPrefix(customID, "sel_chan_panel_") || strings.HasPrefix(customID, "sel_chan_esiti_") || strings.HasPrefix(customID, "sel_role_auth_") {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "✅ **Impostazione salvata con successo!** Il canale o ruolo selezionato è stato collegato correttamente.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	switch customID {
	case "setup_server_select":
		selectedCategory := "cittadinanza"
		if len(i.MessageComponentData().Values) > 0 {
			selectedCategory = i.MessageComponentData().Values[0]
		}

		channelSelectPanel := discordgo.SelectMenu{
			CustomID:    "sel_chan_panel_" + selectedCategory,
			Placeholder: "Seleziona il canale del pannello (#)...",
			MenuType:    discordgo.ChannelSelectMenu,
			ChannelTypes: []discordgo.ChannelType{
				discordgo.ChannelTypeGuildText,
			},
		}

		channelSelectEsiti := discordgo.SelectMenu{
			CustomID:    "sel_chan_esiti_" + selectedCategory,
			Placeholder: "Seleziona il canale degli esiti / log (#)...",
			MenuType:    discordgo.ChannelSelectMenu,
			ChannelTypes: []discordgo.ChannelType{
				discordgo.ChannelTypeGuildText,
			},
		}

		roleSelectAuth := discordgo.SelectMenu{
			CustomID:    "sel_role_auth_" + selectedCategory,
			Placeholder: "Seleziona il ruolo autorizzato (@)...",
			MenuType:    discordgo.RoleSelectMenu,
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("🛠️ **Configurazione per: %s**\nSeleziona qui sotto i canali e i ruoli direttamente dalle liste del server:", strings.ToUpper(selectedCategory[4:])),
				Flags:   discordgo.MessageFlagsEphemeral,
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{channelSelectPanel}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{channelSelectEsiti}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{roleSelectAuth}},
				},
			},
		})

	case "btn_apri_conto":
		selectBanche := discordgo.SelectMenu{
			CustomID:    "select_banca_choice",
			Placeholder: "Seleziona il tuo istituto bancario...",
			Options: []discordgo.SelectMenuOption{
				{Label: "Banca Sella", Value: "banca_sella", Emoji: &discordgo.ComponentEmoji{Name: "🏛️"}},
				{Label: "Poste Italiane", Value: "poste", Emoji: &discordgo.ComponentEmoji{Name: "📮"}},
				{Label: "Intesa Sanpaolo", Value: "intesa", Emoji: &discordgo.ComponentEmoji{Name: "🏦"}},
				{Label: "UniCredit", Value: "unicredit", Emoji: &discordgo.ComponentEmoji{Name: "🔴"}},
				{Label: "BPER Banca", Value: "bper", Emoji: &discordgo.ComponentEmoji{Name: "🟦"}},
				{Label: "Banco BPM", Value: "bpm", Emoji: &discordgo.ComponentEmoji{Name: "🟢"}},
				{Label: "Monte dei Paschi di Siena", Value: "mps", Emoji: &discordgo.ComponentEmoji{Name: "🏰"}},
				{Label: "Credit Agricole Italia", Value: "credit_agricole", Emoji: &discordgo.ComponentEmoji{Name: "🌾"}},
				{Label: "Revolut", Value: "revolut", Emoji: &discordgo.ComponentEmoji{Name: "💳"}},
				{Label: "HYPE (Banca Sella)", Value: "hype", Emoji: &discordgo.ComponentEmoji{Name: "⚡"}},
				{Label: "N26", Value: "n26", Emoji: &discordgo.ComponentEmoji{Name: "🖤"}},
				{Label: "FinecoBank", Value: "fineco", Emoji: &discordgo.ComponentEmoji{Name: "📈"}},
				{Label: "BBVA Italia", Value: "bbva", Emoji: &discordgo.ComponentEmoji{Name: "🟡"}},
				{Label: "Buddybank", Value: "buddybank", Emoji: &discordgo.ComponentEmoji{Name: "📱"}},
			},
		}
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Scegli la banca con cui desideri aprire il conto corrente:",
				Flags:   discordgo.MessageFlagsEphemeral,
				Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{selectBanche}}},
			},
		})

	case "select_banca_choice":
		selectedBanca := "banca_generica"
		if len(i.MessageComponentData().Values) > 0 {
			selectedBanca = i.MessageComponentData().Values[0]
		}
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID: "modal_conferma_conto_" + selectedBanca,
				Title:    "Apertura Conto Corrente",
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "input_nome_cognome_rp", Label: "Nome e Cognome RP", Style: discordgo.TextInputShort, Placeholder: "Es. Mario Rossi", Required: true}}},
				},
			},
		})

	case "btn_richiedi_cittadinanza":
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID: "modal_cittadinanza",
				Title:    "Registrazione Carta d'Identita",
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "input_nome_rp", Label: "Nome RP", Style: discordgo.TextInputShort, Placeholder: "Es. Mario", Required: true}}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "input_cognome_rp", Label: "Cognome RP", Style: discordgo.TextInputShort, Placeholder: "Es. Rossi", Required: true}}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "input_data_nascita", Label: "Data di Nascita RP", Style: discordgo.TextInputShort, Placeholder: "GG/MM/AAAA", Required: true}}},
				},
			},
		})

	case "btn_registra_arresto":
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID: "modal_registra_arresto",
				Title:    "Verbale d'Arresto",
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "input_utente_arrestato", Label: "Utente Roblox Arrestato", Style: discordgo.TextInputShort, Required: true}}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "input_reati", Label: "Reati Commessi", Style: discordgo.TextInputParagraph, Placeholder: "Inseguimento...", Required: true}}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "input_prigione", Label: "Anni o Mesi di Prigione", Style: discordgo.TextInputShort, Required: true}}},
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "input_cauzione", Label: "Cauzione In Euro", Style: discordgo.TextInputShort, Placeholder: "5000", Required: false}}},
				},
			},
		})

	case "btn_riscuoti_stipendio":
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "✅ **Stipendio Riscosso!**", Flags: discordgo.MessageFlagsEphemeral},
		})
	}
}

func handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.ModalSubmitData().CustomID

	if strings.HasPrefix(customID, "modal_conferma_conto_") {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "🎉 **Conto Corrente Aperto con Successo!**", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	switch customID {
	case "modal_cittadinanza":
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "🇮🇹 **Richiesta Cittadinanza Registrata!**", Flags: discordgo.MessageFlagsEphemeral},
		})
	case "modal_registra_arresto":
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "🚨 **Verbale Registrato!**", Flags: discordgo.MessageFlagsEphemeral},
		})
	}
}
