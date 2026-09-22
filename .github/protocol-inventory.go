package main

import (
 "encoding/json"
 "os"
 "sort"
 "github.com/Laisky/one-api/relay"
 "github.com/Laisky/one-api/relay/channeltype"
 "github.com/Laisky/one-api/relay/meta"
)

// main exports configured model capabilities without touching credentials or making provider calls.
func main() {
 rows:=[]map[string]any{}
 for ch:=1;ch<channeltype.Dummy;ch++ {
  a:=relay.GetAdaptor(channeltype.ToAPIType(ch)); if a==nil {continue}
  a.Init(&meta.Meta{ChannelType:ch})
  pricing:=a.GetDefaultModelPricing()
  names:=a.GetModelList(); sort.Strings(names)
  for _,name:=range names {
   cfg:=pricing[name]
   rows=append(rows,map[string]any{"model":name,"channel_type":ch,"provider":a.GetChannelName(),"input_modalities":cfg.InputModalities,"output_modalities":cfg.OutputModalities,"supported_features":cfg.SupportedFeatures,"image_pricing":cfg.Image,"audio_pricing":cfg.Audio,"video_pricing":cfg.Video,"embedding_pricing":cfg.Embedding,"per_call_pricing":cfg.PerCall})
  }
 }
 data,err:=json.MarshalIndent(rows,"","  "); if err!=nil {panic(err)}
 if len(os.Args)!=2 {panic("output path required")}
 if err=os.WriteFile(os.Args[1],data,0600);err!=nil {panic(err)}
}
