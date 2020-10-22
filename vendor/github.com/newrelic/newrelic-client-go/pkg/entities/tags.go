package entities

import (
	"errors"
	"strings"
)

// Tag represents a New Relic One entity tag.
type Tag struct {
	Key    string
	Values []string
}

// TagValue represents a New Relic One entity tag and value pair.
type TagValue struct {
	Key   string
	Value string
}

// ListTags returns a collection of mutable tags for a given entity by entity
// GUID.
func (e *Entities) ListTags(guid string) ([]*Tag, error) {
	resp := listTagsResponse{}
	vars := map[string]interface{}{
		"guid": guid,
	}

	if err := e.client.NerdGraphQuery(listTagsQuery, vars, &resp); err != nil {
		return nil, err
	}

	return filterMutable(resp)
}

// ListAllTags returns a collection of all tags (mutable and not) for a given
// entity by entity GUID.
func (e *Entities) ListAllTags(guid string) ([]*Tag, error) {
	resp := listTagsResponse{}
	vars := map[string]interface{}{
		"guid": guid,
	}

	if err := e.client.NerdGraphQuery(listTagsQuery, vars, &resp); err != nil {
		return nil, err
	}

	return resp.Actor.Entity.Tags, nil
}

// filterMutable removes tag values that are read-only from the received response.
func filterMutable(resp listTagsResponse) ([]*Tag, error) {
	var tags []*Tag

	for _, responseTag := range resp.Actor.Entity.TagsWithMetadata {
		if responseTag != nil {
			tag := Tag{
				Key: responseTag.Key,
			}

			mutable := 0
			for _, responseTagValue := range responseTag.Values {
				if responseTagValue.Mutable {
					mutable++
					tag.Values = append(tag.Values, responseTagValue.Value)
				}
			}

			// All values were mutable
			if len(responseTag.Values) == mutable {
				tags = append(tags, &tag)
			}

		}
	}

	return tags, nil
}

// AddTags writes tags to the entity specified by the provided entity GUID.
func (e *Entities) AddTags(guid string, tags []Tag) error {
	resp := addTagsResponse{}
	vars := map[string]interface{}{
		"guid": guid,
		"tags": tags,
	}

	if err := e.client.NerdGraphQuery(addTagsMutation, vars, &resp); err != nil {
		return err
	}

	if len(resp.TaggingAddTagsToEntity.Errors) > 0 {
		return errors.New(parseTagMutationErrors(resp.TaggingAddTagsToEntity.Errors))
	}

	return nil
}

// ReplaceTags replaces the entity's entire set of tags with the provided tag set.
func (e *Entities) ReplaceTags(guid string, tags []Tag) error {
	resp := replaceTagsResponse{}
	vars := map[string]interface{}{
		"guid": guid,
		"tags": tags,
	}

	if err := e.client.NerdGraphQuery(replaceTagsMutation, vars, &resp); err != nil {
		return err
	}

	if len(resp.TaggingReplaceTagsOnEntity.Errors) > 0 {
		return errors.New(parseTagMutationErrors(resp.TaggingReplaceTagsOnEntity.Errors))
	}

	return nil
}

// DeleteTags deletes specific tag keys from the entity.
func (e *Entities) DeleteTags(guid string, tagKeys []string) error {
	resp := deleteTagsResponse{}
	vars := map[string]interface{}{
		"guid":    guid,
		"tagKeys": tagKeys,
	}

	if err := e.client.NerdGraphQuery(deleteTagsMutation, vars, &resp); err != nil {
		return err
	}

	if len(resp.TaggingDeleteTagFromEntity.Errors) > 0 {
		return errors.New(parseTagMutationErrors(resp.TaggingDeleteTagFromEntity.Errors))
	}

	return nil
}

// DeleteTagValues deletes specific tag key and value pairs from the entity.
func (e *Entities) DeleteTagValues(guid string, tagValues []TagValue) error {
	resp := deleteTagValuesResponse{}
	vars := map[string]interface{}{
		"guid":      guid,
		"tagValues": tagValues,
	}

	if err := e.client.NerdGraphQuery(deleteTagValuesMutation, vars, &resp); err != nil {
		return err
	}

	if len(resp.TaggingDeleteTagValuesFromEntity.Errors) > 0 {
		return errors.New(parseTagMutationErrors(resp.TaggingDeleteTagValuesFromEntity.Errors))
	}

	return nil
}

type tagMutationError struct {
	Type    string
	Message string
}

func parseTagMutationErrors(errors []tagMutationError) string {
	messages := []string{}
	for _, e := range errors {
		messages = append(messages, e.Message)
	}

	return strings.Join(messages, ", ")
}

// EntityTagValueWithMetadata - The value and metadata of a single entity tag.
type EntityTagValueWithMetadata struct {
	// Whether or not the tag can be mutated by the user.
	Mutable bool `json:"mutable"`
	// The tag value.
	Value string `json:"value"`
}

// EntityTagWithMetadata - The tags with metadata of the entity.
type EntityTagWithMetadata struct {
	// The tag's key.
	Key string `json:"key"`
	// A list of tag values with metadata information.
	Values []EntityTagValueWithMetadata `json:"values"`
}

var listTagsQuery = `
	query($guid:EntityGuid!) { actor { entity(guid: $guid)  {` +
	graphqlEntityStructTagsFields +
	` } } }`

type listTagsResponse struct {
	Actor struct {
		Entity struct {
			Tags             []*Tag
			TagsWithMetadata []*EntityTagWithMetadata
		}
	}
}

var addTagsMutation = `
	mutation($guid: EntityGuid!, $tags: [TaggingTagInput!]!) {
		taggingAddTagsToEntity(guid: $guid, tags: $tags) {
			errors {
				type
				message
			}
		}
	}
`

type addTagsResponse struct {
	TaggingAddTagsToEntity struct {
		Errors []tagMutationError
	}
}

var replaceTagsMutation = `
	mutation($guid: EntityGuid!, $tags: [TaggingTagInput!]!) {
		taggingReplaceTagsOnEntity(guid: $guid, tags: $tags) {
			errors {
				type
				message
			}
		}
	}
`

type replaceTagsResponse struct {
	TaggingReplaceTagsOnEntity struct {
		Errors []tagMutationError
	}
}

var deleteTagsMutation = `
	mutation($guid: EntityGuid!, $tagKeys: [String!]!) {
		taggingDeleteTagFromEntity(guid: $guid, tagKeys: $tagKeys) {
			errors {
				type
				message
			}
		}
	}
`

type deleteTagsResponse struct {
	TaggingDeleteTagFromEntity struct {
		Errors []tagMutationError
	}
}

var deleteTagValuesMutation = `
	mutation($guid: EntityGuid!, $tagValues: [TaggingTagValueInput!]!) {
		taggingDeleteTagValuesFromEntity(guid: $guid, tagValues: $tagValues) {
			errors {
				type
				message
			}
		}
	}
`

type deleteTagValuesResponse struct {
	TaggingDeleteTagValuesFromEntity struct {
		Errors []tagMutationError
	}
}
