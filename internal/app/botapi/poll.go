package botapi

import (
	"encoding/json"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

type pollData []string

func (p *pollData) UnmarshalJSON(data []byte) error {
	var stringsValue []string
	if err := json.Unmarshal(data, &stringsValue); err == nil {
		*p = append((*p)[:0], stringsValue...)
		return nil
	}

	var options []domainbot.PollOption
	if err := json.Unmarshal(data, &options); err != nil {
		return err
	}

	values := make([]string, 0, len(options))
	for _, option := range options {
		values = append(values, option.Text)
	}
	*p = values

	return nil
}
