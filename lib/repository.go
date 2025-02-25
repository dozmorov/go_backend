package lib

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"log"
	"os"
	"time"
)

type Repository struct {
	Server   string
	Database string
	Session  *mgo.Session
}

const SALT = "1c2cf9a0a9031262b894fac41f05e656"
const OPENBEFORE = 60 * 45
const OPENAFTER = 60 * 60 * 15
const BLOCKAFETRENTRY = 60 * 60 * 2
const USER_COLLECTION = "users"
const TERMINALS_COLLECTION = "terminals"
const GROUPS_COLLECTION = "groups"
const TICKETS_COLLECTION = "tickets"
const EVENTS_COLLECTION = "events"
const ENTRY_COLLECTION = "entry"
const LOGS_COLLECTION = "logs"
const MASTERKEY_COLLECTION = "masterkey"

var db *mgo.Database
var ticketsLocked TicketsLocked
var masterKeys MasterKeys

func NullIsNow(t int64) int64 {
	if t == 0 {
		return time.Now().Unix()
	} else {
		return t
	}
}

func (r *Repository) Connect() {
	r.Server = os.Getenv("MONGO_URL")
	r.Database = os.Getenv("MONGO_DB")
	var err error
	r.Session, err = mgo.Dial(r.Server)
	if err != nil {
		log.Fatal(err)
	}
	r.Session.SetMode(mgo.Eventual, false)
	log.Println("Connected to ", r.Server, "with", r.Database, "database.")
	db = r.Session.DB(r.Database)
	// Optional. Switch the session to a monotonic behavior.
	r.LoadMasterKeys()
	//r.GenDemoData(1,600, 1,"demo")

}
func getResultForEntry(entryItem Entry) (entry bool, exit bool) {
	if entryItem == (Entry{}) || entryItem.Direction == "exit" {
		return true, false
	}
	return false, true
}
func hashPassword(pass string) string {
	hash := md5.New()
	hash.Write([]byte(pass + SALT))
	return hex.EncodeToString(hash.Sum(nil))
}
func genSecretKey() string {
	hash := md5.New()
	hash.Write([]byte(time.Now().String() + SALT))
	return hex.EncodeToString(hash.Sum(nil))
}

func (r *Repository) Maintenance() {
	r.MaintenceActiveEvents(60)
	r.SyncAllGroupsEvents()

}

func (r *Repository) GenDemoData(startNum int, endNum int, eventId int, source string) {
	timeUnix := time.Now().Unix()
	var event Event
	event.Id = int64(eventId)
	event.Title = source
	event.VenueTitle = source
	event.VenueId = 603
	event.LastUpdate = timeUnix
	event.EventDT = timeUnix

	r.Session.DB(r.Database).C(EVENTS_COLLECTION).Upsert(bson.M{"event_id": eventId}, event)
	bulk := r.Session.DB(r.Database).C(TICKETS_COLLECTION).Bulk()
	for i := 1; i <= 600; i++ {
		var element TicketExport
		element.EventID = eventId
		element.TicketSector = source
		element.TicketID = i
		element.TicketBarcode = fmt.Sprintf("%012d", i)
		element.LastUpdate = timeUnix
		element.TicketDt = int(timeUnix)
		element.Source = source
		bulk.Upsert(bson.M{"ticket_id": element.TicketID, "event_id": element.EventID}, element)
	}
	_, err := bulk.Run()

	if err == nil {
		r.Session.DB(r.Database).C(TICKETS_COLLECTION).RemoveAll(bson.M{"event_id": eventId, "source": source, "last_update": bson.M{"$lt": timeUnix}})
	}
}

func (r *Repository) CheckUser(userLogin UserLogin) (bool, *Exception) {
	//result := &User{}
	//db.C("users").Insert(&User{"tester","just test"})
	userCount, errFind := db.C("users").Find(bson.M{"active": true, "login": userLogin.Login, "password": hashPassword(userLogin.Password)}).Count()
	if errFind != nil {
		return false, &Exception{CANT_SELECT_EXEPTION, errFind.Error()}
	}
	//Correct user
	if userCount == 1 {
		return true, nil
	}
	//Not found
	return false, nil
}
func (r *Repository) Terminals() interface{} {
	/*	query := []bson.M{{
			"$lookup": bson.M{
				"from": "groups",
				"localField": "groups",
				"foreignField": "id",
				"as": "groups",
			}}}
		terms :=[]Terminal{}
		pipe := db.C(TERMINALS_COLLECTION).Pipe(query)
		pipe.All(&terms)*/
	var terms []Terminal
	db.C(TERMINALS_COLLECTION).Find(nil).All(&terms)

	return Terminals{terms}
}
func (r *Repository) Groups() interface{} {
	var result []Group
	db.C(GROUPS_COLLECTION).Find(nil).All(&result)

	return Groups{result}
}
func (r *Repository) AddUser(user User) *Exception {
	//Try to find user
	userCount, errFind := db.C(USER_COLLECTION).Find(bson.M{"login": user.Login}).Count()
	if errFind != nil {
		return &Exception{CANT_SELECT_EXEPTION, errFind.Error()}
	}
	if userCount > 0 {
		return &Exception{USER_EXIST_EXEPTION, ""}
	}
	user.Password = hashPassword(user.Password)
	user.Active = true
	errInsert := db.C(USER_COLLECTION).Insert(user)
	if errInsert != nil {
		return &Exception{CANT_INSERT_EXEPTION, errInsert.Error()}
	}
	return nil
}
func (r *Repository) SetGroup(group Group) *Exception {

	db.C(GROUPS_COLLECTION).Upsert(bson.M{"name": group.Name}, group)
	log.Println(group)
	return nil
}

func (r *Repository) SetTerminal(terminal Terminal) *Exception {
	log.Println(terminal)
	db.C(TERMINALS_COLLECTION).Update(bson.M{"id": terminal.Id}, bson.M{"$set": terminal})
	return nil
}
func (r *Repository) AddTerminal(terminal Terminal) *Exception {

	terminalCount, errFind := db.C(TERMINALS_COLLECTION).Find(bson.M{"name": terminal.Name}).Count()
	if errFind != nil {
		return &Exception{CANT_SELECT_EXEPTION, errFind.Error()}
	}
	if terminalCount > 0 {
		return &Exception{TERMINAL_EXIST_EXEPTION, ""}
	}

	//find max Id
	var trm Terminal
	db.C(TERMINALS_COLLECTION).Find(nil).Sort("-id").One(&trm)
	terminal.Id = trm.Id + 1
	terminal.Secret = genSecretKey()
	errInsert := db.C(TERMINALS_COLLECTION).Insert(terminal)
	if errInsert != nil {
		return &Exception{CANT_INSERT_EXEPTION, errInsert.Error()}
	}
	return nil
}
func (r *Repository) AddGroup(group Group) *Exception {
	//Try to find user
	groupCount, errFind := db.C(GROUPS_COLLECTION).Find(bson.M{"name": group.Name}).Count()
	if errFind != nil {
		return &Exception{CANT_SELECT_EXEPTION, errFind.Error()}
	}
	if groupCount > 0 {
		return &Exception{USER_EXIST_EXEPTION, ""}
	}
	//find max Id
	var grp Group
	db.C(GROUPS_COLLECTION).Find(nil).Sort("-id").One(&grp)
	group.Id = grp.Id + 1

	errInsert := db.C(GROUPS_COLLECTION).Insert(group)
	if errInsert != nil {
		return &Exception{CANT_INSERT_EXEPTION, errInsert.Error()}
	}
	return nil
}

func (r *Repository) AddMasterKey(key MasterKey) *Exception {
	//Try to find user
	keycount, errFind := db.C(MASTERKEY_COLLECTION).Find(bson.M{"barcode": key.Barcode}).Count()
	if errFind != nil {
		return &Exception{CANT_SELECT_EXEPTION, errFind.Error()}
	}
	if keycount > 0 {
		return &Exception{MASTERKEY_EXIST_EXEPTION, ""}
	}
	errInsert := db.C(MASTERKEY_COLLECTION).Insert(key)
	if errInsert != nil {
		return &Exception{CANT_INSERT_EXEPTION, errInsert.Error()}
	}
	return nil
}
func (r *Repository) LoadMasterKeys() {
	//Try to find user
	db.C(MASTERKEY_COLLECTION).Find(nil).All(&masterKeys.keys)
}

func (r *Repository) Log(log Log) *Exception {
	log.Dt = time.Now().Unix()
	errInsert := db.C(LOGS_COLLECTION).Insert(log)
	if errInsert != nil {
		return &Exception{CANT_INSERT_EXEPTION, errInsert.Error()}
	}
	return nil
}
func (r *Repository) Logs() []Log {
	var logs []Log
	db.C(LOGS_COLLECTION).Find(nil).All(&logs)
	return logs
}
func (r *Repository) RemoveGroup(group Group) *Exception {

	db.C(GROUPS_COLLECTION).Remove(group)

	return nil
}

func (r *Repository) SyncAllGroupsEvents() *Exception {
	groups := Groups{}
	db.C(GROUPS_COLLECTION).Find(nil).All(&groups.Groups)
	for _, element := range groups.Groups {
		r.SyncEventsList(element.BuildingId)
	}
	return nil
}

func (r *Repository) SyncEventsList(buildingId int64) {
	pageEvents := api.PageEventList(buildingId, time.Now().Add(-time.Second*60*60*24).Unix(), time.Now().Add(time.Second*60*60*24*90).Unix())
	r.AddEvents(pageEvents.ToEvents())
}
func (r *Repository) AddEvents(events Events) *Exception {

	bulk := db.C(EVENTS_COLLECTION).Bulk()
	timeUnix := time.Now().Unix()
	for _, element := range events.Events {
		element.LastUpdate = timeUnix
		bulk.Upsert(bson.M{"event_id": element.Id}, element)
	}
	bulk.Run()
	return nil
	/*	//remove old items
		if err == nil {
			db.C(EVENTS_COLLECTION).RemoveAll(bson.M{"event_id": eventExport.Content.Data.Event.EventID, "source": source, "last_update": bson.M{"$lt": timeUnix}})
		}
		return nil*/
}

func (r *Repository) SyncAllEvents() *Exception {
	events := Events{}
	db.C(EVENTS_COLLECTION).Find(nil).All(&events.Events)
	//sync Tickets
	for _, element := range events.Events {
		r.SyncEvent(element.Id)
	}
	return nil
}

func (r *Repository) SyncEvent(eventId int64) (Event, *Exception) {
	session := r.Session.Clone()
	defer session.Close()

	eventExport := api.GetEventACS(eventId)
	timeUnix := time.Now().Unix()
	eventExport.Content.Data.Event.LastUpdate = timeUnix
	//sync Event
	session.DB(r.Database).C(EVENTS_COLLECTION).Upsert(bson.M{"event_id": eventExport.Content.Data.Event.EventID}, eventExport.Content.Data.Event)
	//sync Tickets
	bulk := session.DB(r.Database).C(TICKETS_COLLECTION).Bulk()
	source := api.Source()
	for _, element := range eventExport.Content.Data.Event.Tickets {
		element.EventID = eventExport.Content.Data.Event.EventID
		element.LastUpdate = timeUnix
		element.Source = source
		bulk.Upsert(bson.M{"ticket_id": element.TicketID, "event_id": element.EventID}, element)
	}
	_, err := bulk.Run()
	//log.Println("delete old")
	//remove old items
	if err == nil {
		session.DB(r.Database).C(TICKETS_COLLECTION).RemoveAll(bson.M{"event_id": eventExport.Content.Data.Event.EventID, "source": source, "last_update": bson.M{"$lt": timeUnix}})
	}
	var event Event
	session.DB(r.Database).C(EVENTS_COLLECTION).Find(bson.M{"event_id": eventId}).One(&event)
	event.TicketsCached = r.GetTicketsCountByEvent(event)
	return event, nil
}

func (r *Repository) ValidateTicket(barcode string, term Terminal) (SKDResponse, *Exception) {
	curentGroups := r.GetGroupsByTerminal(term)
	currentEvents := r.GetActiveEventsByGroups(curentGroups)
	ticket := Ticket{}
	db.C(TICKETS_COLLECTION).Find(bson.M{"ticket_barcode": barcode, "event_id": bson.M{"$in": currentEvents.EventsIds()}}).One(&ticket)

	if (Ticket{}) != ticket {
		entry := r.CheckTicketForEntry(ticket)

		return SKDResponse{SKDResult{ENTRY_RESULT_CODE_ACCEPT}, ticket, currentEvents.EventById(ticket.EventId), entry.toAction()}, nil
	}
	//Not Found
	ticket.TicketBarcode = barcode
	return SKDResponse{SKDResult{ENTRY_RESULT_CODE_NOTFOUND}, ticket, Event{}, Action{}}, nil
}

func (r *Repository) ValidateRegistrateTicket(barcode string, term Terminal, direction string) (SKDRegistrationResponse, *Exception) {
	if masterKeys.is(barcode) { //Master key
		return SKDRegistrationResponse{SKDRegistrationResult{ENTRY_RESULT_CODE_ACCEPT, direction == "entry", direction == "exit"}, Ticket{}, Event{}, Action{}}, nil
	}
	curentGroups := r.GetGroupsByTerminal(term)
	currentEvents := r.GetActiveEventsByGroups(curentGroups)
	ticket := Ticket{}
	db.C(TICKETS_COLLECTION).Find(bson.M{"ticket_barcode": barcode, "event_id": bson.M{"$in": currentEvents.EventsIds()}}).One(&ticket)

	// 
	log.Println("TERMINAL:", term.Name, direction, "TICKET:", ticket.TicketBarcode, "EVENT:", ticket.TicketTitle, ticket.TicketSector, "PRICE:", ticket.TicketPrice)

	if (Ticket{}) != ticket {
		entryItem := r.CheckTicketForEntry(ticket)
		entry, exit := getResultForEntry(entryItem)
		if ticketsLocked.isLock(barcode) || (r.TicketEntryFirstTime(ticket)+BLOCKAFETRENTRY < time.Now().Unix()) {
			log.Println("!!!")
			log.Println("lock", entryItem)
			log.Println(time.Now().Unix() - BLOCKAFETRENTRY)
			//Reenty for LockTicket OR Block entry after expiration
			entryRecord := Entry{ticket.EventId, ticket.TicketBarcode, term.Id, time.Now().Unix(), ENTRY_RESULT_CODE_REENTRY, direction}
			errInsert := db.C(ENTRY_COLLECTION).Insert(entryRecord)
			if errInsert != nil {
				return SKDRegistrationResponse{}, &Exception{CANT_INSERT_EXEPTION, errInsert.Error()}
			}
			return SKDRegistrationResponse{SKDRegistrationResult{ENTRY_RESULT_CODE_REENTRY, false, false}, ticket, currentEvents.EventById(ticket.EventId), entryItem.toAction()}, nil
		}
		if entry && direction == "entry" {
			//Entry allowed
			ticketsLocked.addLock(barcode)
			return SKDRegistrationResponse{SKDRegistrationResult{ENTRY_RESULT_CODE_ACCEPT, entry, exit}, ticket, currentEvents.EventById(ticket.EventId), entryItem.toAction()}, nil
		}
		if exit && direction == "exit" {
			//Exit
			ticketsLocked.addLock(barcode)
			return SKDRegistrationResponse{SKDRegistrationResult{ENTRY_RESULT_CODE_ACCEPT, entry, exit}, ticket, currentEvents.EventById(ticket.EventId), entryItem.toAction()}, nil
		}

		//reentry
		entryRecord := Entry{ticket.EventId, ticket.TicketBarcode, term.Id, time.Now().Unix(), ENTRY_RESULT_CODE_REENTRY, direction}
		errInsert := db.C(ENTRY_COLLECTION).Insert(entryRecord)
		if errInsert != nil {
			log.Println("error")
			return SKDRegistrationResponse{}, &Exception{CANT_INSERT_EXEPTION, errInsert.Error()}
		}

		log.Println("reentry?")
		return SKDRegistrationResponse{SKDRegistrationResult{ENTRY_RESULT_CODE_REENTRY, false, false}, ticket, currentEvents.EventById(ticket.EventId), entryItem.toAction()}, nil
	}
	log.Println("1.2")
	//Not Found
	ticket.TicketBarcode = barcode
	return SKDRegistrationResponse{SKDRegistrationResult{ENTRY_RESULT_CODE_NOTFOUND, false, false}, ticket, Event{}, Action{}}, nil
}

func (r *Repository) RegistrateTicket(barcode string, term Terminal, direction string) (SKDResult, *Exception) {
	if masterKeys.is(barcode) { //Master key
		return SKDResult{ENTRY_RESULT_CODE_ACCEPT}, nil
	}
	curentGroups := r.GetGroupsByTerminal(term)
	currentEvents := r.GetActiveEventsByGroups(curentGroups)
	ticket := Ticket{}
	db.C(TICKETS_COLLECTION).Find(bson.M{"ticket_barcode": barcode, "event_id": bson.M{"$in": currentEvents.EventsIds()}}).One(&ticket)

	if (Ticket{}) != ticket {
		entryItem := r.CheckTicketForEntry(ticket)
		entry, exit := getResultForEntry(entryItem)
		fmt.Println(entry, exit)
		if entry && direction == "entry" {
			//Entry allowed
			entryRecord := Entry{ticket.EventId, ticket.TicketBarcode, term.Id, time.Now().Unix(), ENTRY_RESULT_CODE_ACCEPT, direction}
			errInsert := db.C(ENTRY_COLLECTION).Insert(entryRecord)
			if errInsert != nil {
				return SKDResult{ENTRY_RESULT_CODE_NOTFOUND}, &Exception{CANT_INSERT_EXEPTION, errInsert.Error()}
			}
			return SKDResult{ENTRY_RESULT_CODE_ACCEPT}, nil
		}
		if exit && direction == "exit" {
			//Exit
			entryRecord := Entry{ticket.EventId, ticket.TicketBarcode, term.Id, time.Now().Unix(), ENTRY_RESULT_CODE_ACCEPT, direction}
			errInsert := db.C(ENTRY_COLLECTION).Insert(entryRecord)
			if errInsert != nil {
				return SKDResult{ENTRY_RESULT_CODE_NOTFOUND}, &Exception{CANT_INSERT_EXEPTION, errInsert.Error()}
			}
			return SKDResult{ENTRY_RESULT_CODE_ACCEPT}, nil
		}
		//reentry
		entryRecord := Entry{ticket.EventId, ticket.TicketBarcode, term.Id, time.Now().Unix(), ENTRY_RESULT_CODE_REENTRY, direction}
		errInsert := db.C(ENTRY_COLLECTION).Insert(entryRecord)
		if errInsert != nil {
			return SKDResult{ENTRY_RESULT_CODE_NOTFOUND}, &Exception{CANT_INSERT_EXEPTION, errInsert.Error()}
		}
		return SKDResult{ENTRY_RESULT_CODE_NOTFOUND}, nil
	}
	//Not Found
	return SKDResult{ENTRY_RESULT_CODE_NOTFOUND}, nil
}
func (r *Repository) GetGroupsByTerminal(terminal Terminal) Groups {
	groups := Groups{}
	db.C(GROUPS_COLLECTION).Find(bson.M{"id": bson.M{"$in": terminal.Groups}}).All(&groups.Groups)
	return groups
}
func (r *Repository) GetActiveEventsByGroups(groups Groups) Events {
	timeUnix := time.Now().Unix()
	events := Events{}
	db.C(EVENTS_COLLECTION).Find(bson.M{"event_dt": bson.M{"$lte": timeUnix + OPENBEFORE, "$gte": timeUnix - OPENAFTER}, "venue_id": bson.M{"$in": groups.BildingsIds()}, "hall_id": bson.M{"$nin": groups.ExcludeIds()}}).All(&events.Events)
	return events
}
func (r *Repository) MaintenceActiveEvents(deltaDt int64) Events {
	timeUnix := time.Now().Unix()
	events := Events{}
	db.C(EVENTS_COLLECTION).Find(bson.M{"event_dt": bson.M{"$lte": timeUnix + OPENBEFORE + deltaDt, "$gte": timeUnix - OPENAFTER - deltaDt}}).All(&events.Events)
	for _, event := range events.Events {
		r.SyncEvent(event.Id)
	}
	return events
}
func (r *Repository) GetEventById(id int64) Event {
	event := Event{}
	db.C(EVENTS_COLLECTION).Find(bson.M{"event_id": id}).One(&event)
	return event
}
func (r *Repository) GetEventInfo(id int64) EventInfo {
	var tickets, entrys []bson.M
	eventInfo := EventInfo{}
	pipeTickets := db.C(TICKETS_COLLECTION).Pipe([]bson.M{
		bson.M{"$match": bson.M{"event_id": id}},
		bson.M{"$group": bson.M{"_id": "$ticket_price", "count": bson.M{"$sum": 1}}}})
	pipeEntry := db.C(ENTRY_COLLECTION).Pipe([]bson.M{
		bson.M{"$match": bson.M{"event_id": id, "result_code": 1, "direction": "entry"}},
		bson.M{"$group": bson.M{"_id": "$ticket_barcode"}},
		bson.M{"$lookup": bson.M{"from": "tickets", "localField": "_id", "foreignField": "ticket_barcode", "as": "ticket"}},
		bson.M{"$unwind": "$ticket"},
		bson.M{"$group": bson.M{"_id": "$ticket.ticket_price", "count": bson.M{"$sum": 1}}}})

	pipeTickets.All(&tickets)
	pipeEntry.All(&entrys)

	eventInfo.fromPriceMap(tickets, entrys)
	//pipeEntry := db.C(ENTRY_COLLECTION).Pipe([]bson.M{{"$match": bson.M{"event_id": id, "direction": "entry", "result_code": 1}}, {"$group": bson.M{"_id": "$ticket_barcode"}}, {"$count": "entries"}})
	//resp := EventInfo{}
	//pipeTickets.One(&resp.Tickets)
	//pipeEntry.One(&resp.Entries)
	return eventInfo
}
func (r *Repository) GetEventsByGroup(groupId int64) Events {
	group := Group{}
	events := Events{}
	db.C(GROUPS_COLLECTION).Find(bson.M{"id": groupId}).One(&group)
	db.C(EVENTS_COLLECTION).Find(bson.M{"venue_id": group.BuildingId, "hall_id": bson.M{"$nin": group.Exclude_halls}}).All(&events.Events)
	for i, event := range events.Events {
		//event.TicketsCached = r.GetTicketsCountByEvent(event)
		events.Events[i] = event
	}
	return events
}
func (r *Repository) GetEventsByDt(dtf, dtt int64) Events {
	events := Events{}
	db.C(EVENTS_COLLECTION).Find(bson.M{"event_dt": bson.M{"$lte": dtt, "$gte": dtf}}).All(&events.Events)
	return events
}
func (r *Repository) GetTicketsCountByEvent(event Event) int {

	ticketsCount, _ := db.C(TICKETS_COLLECTION).Find(bson.M{"event_id": event.Id}).Count()
	return ticketsCount
}
func (r *Repository) GetTerminalById(terminalId int64) Terminal {
	term := Terminal{}
	db.C(TERMINALS_COLLECTION).Find(bson.M{"id": terminalId}).One(&term)
	return term
}
func (r *Repository) GetAuthTerminalById(terminalId int64) AuthStruct {
	term := AuthStruct{}
	db.C(TERMINALS_COLLECTION).Find(bson.M{"id": terminalId}).One(&term.Auth)
	//fix for back compatibility
	term.Terminal.Title = term.Auth.Title
	term.Terminal.ID = term.Auth.ID
	return term
}
func (r *Repository) CheckTicketForEntry(ticket Ticket) Entry {
	entry := Entry{}
	db.C(ENTRY_COLLECTION).Find(bson.M{"ticket_barcode": ticket.TicketBarcode, "event_id": ticket.EventId, "result_code": ENTRY_RESULT_CODE_ACCEPT}).Sort("-operation_dt").One(&entry)
	return entry
}
func (r *Repository) TicketEntryFirstTime(ticket Ticket) int64 {
	entry := Entry{}
	db.C(ENTRY_COLLECTION).Find(bson.M{"ticket_barcode": ticket.TicketBarcode, "event_id": ticket.EventId, "result_code": ENTRY_RESULT_CODE_ACCEPT}).Sort("operation_dt").One(&entry)
	return NullIsNow(entry.OperationDt)
}
func (r *Repository) CheckTicket(check CheckTiket) CheckResult {
	ticket := Ticket{}
	var entry []bson.M
	db.C(TICKETS_COLLECTION).Find(bson.M{"ticket_barcode": check.Barcode}).One(&ticket)
	pipeEntry := db.C(ENTRY_COLLECTION).Pipe([]bson.M{
		bson.M{"$match": bson.M{"ticket_barcode": check.Barcode}},
		bson.M{"$lookup": bson.M{"from": "terminals", "localField": "terminal_id", "foreignField": "id", "as": "term"}},
		bson.M{"$unwind": "$term"}})
	pipeEntry.All(&entry)

	//db.C(ENTRY_COLLECTION).Find(bson.M{"ticket_barcode": check.Barcode}).Sort("operation_dt").All(&entry)

	fmt.Println(entry)
	event := r.GetEventById(ticket.EventId)
	ticket.TicketBarcode = check.Barcode
	return CheckResult{event, ticket, entry}
}
