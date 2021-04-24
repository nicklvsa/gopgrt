package pkg

import "encoding/json"

func UnmarshalInterface(content, intoModel interface{}) error {
	data, err := json.Marshal(content)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, &intoModel)
	if err != nil {
		return err
	}

	return nil
}
