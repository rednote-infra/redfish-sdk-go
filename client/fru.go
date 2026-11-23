package client

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/pointer"
)

func (c *Client) GetFRUService(ctx context.Context, chassisID ...string) ([]Object, error) {
	chassis, err := c.GetChassis(ctx, chassisID...)
	if err != nil {
		return nil, err
	}
	if chassis.OEM == nil {
		return []Object{}, nil
	}
	uri, err := findOEMLink(*chassis.OEM, "FruService")
	if errors.Is(err, sdkerrors.ErrNotFound) {
		uri, err = findOEMLink(*chassis.OEM, "Fru")
	}
	if errors.Is(err, sdkerrors.ErrNotFound) {
		return []Object{}, nil
	}
	if err != nil {
		return nil, err
	}
	return collection[Object](ctx, c, uri)
}

func (c *Client) GetBaseboardFRU(ctx context.Context, chassisID ...string) (Object, error) {
	items, err := c.GetFRUService(ctx, chassisID...)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		uri, _ := item["@odata.id"].(string)
		if strings.HasSuffix(uri, "/0") || strings.HasSuffix(strings.ToLower(uri), "mainboardid0") {
			return item, nil
		}
	}
	return nil, nil
}

type fruInfoEntry struct {
	Board   *models.MainBoard  `json:"Board"`
	Chassis *models.FruChassis `json:"Chassis"`
	Product *models.FruProduct `json:"Product"`
}

func (c *Client) GetSystemFRU(ctx context.Context, systemID ...string) (*models.Fru, error) {
	system, err := c.GetSystem(ctx, systemID...)
	if err != nil {
		return nil, err
	}
	if system.OEM != nil {
		uri, err := findOEMLink(*system.OEM, "FruInfo")
		if err == nil {
			var info struct {
				Entries map[string]fruInfoEntry `json:"FruInfo"`
			}
			if err = c.Get(ctx, uri, &info); err != nil {
				return nil, err
			}
			keys := make([]string, 0, len(info.Entries))
			for key := range info.Entries {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			for _, wanted := range []string{"MB_FRU", "MAINBOARD", "MB", ""} {
				for _, key := range keys {
					entry := info.Entries[key]
					if entry.Board != nil && strings.Contains(strings.ToUpper(key), wanted) {
						if pointer.Value(entry.Board.ProductName) == "" && entry.Product != nil {
							entry.Board.ProductName = entry.Product.Name
						}
						return &models.Fru{ODataID: pointer.Ptr(uri), Board: entry.Board, Chassis: entry.Chassis, Product: entry.Product}, nil
					}
				}
			}
		}
		if !errors.Is(err, sdkerrors.ErrNotFound) {
			return nil, err
		}
		uri, err = findOEMLink(*system.OEM, "Fru")
		if err == nil {
			return getResource[models.Fru](ctx, c, uri)
		}
		if !errors.Is(err, sdkerrors.ErrNotFound) {
			return nil, err
		}
	}
	if system.Links != nil {
		for _, link := range system.Links.Chassis {
			items, err := c.GetFRUService(ctx, pointer.Value(link.ODataID))
			if err != nil {
				if errors.Is(err, sdkerrors.ErrNotFound) {
					continue
				}
				return nil, err
			}
			for _, raw := range items {
				uri, _ := raw["@odata.id"].(string)
				if !strings.HasSuffix(uri, "/0") && !strings.HasSuffix(strings.ToLower(uri), "mainboardid0") {
					continue
				}
				resource, err := getResource[models.XFusionFru](ctx, c, uri)
				if err != nil {
					return nil, err
				}
				if rawBoard := resource.Extra["BoardInfo"]; rawBoard != nil {
					var board models.MainBoard
					if err = json.Unmarshal(rawBoard, &board); err != nil {
						return nil, err
					}
					return &models.Fru{ODataID: resource.ODataID, Board: &board}, nil
				}
				board := &models.MainBoard{Manufacturer: resource.BoardMfg, ProductName: resource.BoardProduct, SerialNumber: resource.BoardSerial, PartNumber: resource.BoardPartNumber, BuildDate: resource.BoardMfgDate, ChassisPartNumber: resource.ChassisPartNumber}
				return &models.Fru{ODataID: resource.ODataID, Board: board, Chassis: &models.FruChassis{ChassisPartNumber: resource.ChassisPartNumber, ChassisSerialNumber: resource.ChassisSerial, ChassisType: resource.ChassisType}, Product: &models.FruProduct{Manufacturer: resource.ProductManufacturer, Name: resource.ProductName, PartNumber: resource.ProductPartNumber, SerialNumber: resource.ProductSerial, Version: resource.ProductVersion, AssetTag: resource.ProductAssetTag}}, nil
			}
		}
	}
	return nil, nil
}

func (c *Client) GetMainboard(ctx context.Context, systemID, chassisID string) (*models.MainBoard, error) {
	fru, err := c.GetSystemFRU(ctx, systemID)
	if err == nil && fru != nil && fru.Board != nil {
		return fru.Board, nil
	}
	if err != nil && !errors.Is(err, sdkerrors.ErrNotFound) {
		return nil, err
	}
	raw, err := c.GetBaseboardFRU(ctx, chassisID)
	if err != nil && !errors.Is(err, sdkerrors.ErrNotFound) {
		return nil, err
	}
	if raw != nil {
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, err
		}
		var view struct {
			BoardInfo *models.MainBoard `json:"BoardInfo"`
		}
		if err = json.Unmarshal(data, &view); err != nil {
			return nil, err
		}
		if view.BoardInfo != nil {
			return view.BoardInfo, nil
		}
		var board models.MainBoard
		if err = json.Unmarshal(data, &board); err != nil {
			return nil, err
		}
		return &board, nil
	}
	chassis, err := c.GetChassis(ctx, chassisID)
	if err != nil {
		return nil, err
	}
	if chassis.OEM != nil {
		uri, err := findOEMLink(*chassis.OEM, "MainBoard")
		if err == nil {
			return getResource[models.MainBoard](ctx, c, uri)
		}
		if !errors.Is(err, sdkerrors.ErrNotFound) {
			return nil, err
		}
	}
	return nil, nil
}
