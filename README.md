# Otter-AI - Governed AI Agent System
                                                                                                    
                                                                      **                            
                                                                 :[): )#+  []                       
                                                                  ) *}] *<=>[-                      
                                                               :*<)*  :  -}=-)                      
                                                           :*]=              -[-                    
                                                    *[<-:<%-                   +[                   
                                                 :}%]>- ]+                       *<                 
                                                  <*  =[                           )*               
                                                    >}                               [=             
                                                  +>:   <}-                           +#<><]+       
                                                -)      :*                             -]:   *)=    
                                               ]>        :=++=+=:                       +%= =<#@[:  
                                     +)>>**><<[}]]*:   :: :[@@@@@@@@%}<:    <#)          #:    >):  
                                     -*><))]][%%}<>>>*      -}@@@@@@@@@[)<- =)-         :@]]]]+     
                                   ::      :-)@@%)<>=          *@%[<-                   )=          
                                       :]]*-: =:[       +[=  :<}#-                     +>           
                                     *)-        <=         ---   )<:   :    -+-        -)           
                                                 }%[]]#####[-      -<]):  >}<-: +][*-  -]           
                                                <+ ]  ]:    =}-           :>}*->[):  -*#}           
                                               :) -* =[  :]  -#*:             +)* :*)<=<++<+        
                                            -=*#}=:  )  :]:  :+ >#+              *]-=#[#>:  :       
                                         )[-:     -}:* +)   [*    <[              :%@@<  -]:        
                                       )>        <]=#  =  ><  =)-  @:  =]}]<>>><)]%= *)    -*       
                                     -[   -    :  :[%=   ]:  ::   <*    )-        +]   ]-           
                                    :[    <@   :%+ :#  =}  <%[[}[%[     [:         ]-   =           
                                    ]*>]>= -[:  :#)<   + ]]>>)=   :+]*  }:         *>               
                                   +}+       *]][<<]-  -%---         :[][:         -#               
                            -]*  -#:                -)[#@=  =<:        :%+          }-              
                         *[* =}#}]:                    -%:               <]         ]=              
                       *#- +#+=[}}*     :+)}}@%[:        [>:              ++        [=              
                      #= -%=-#: -%@:  -@% :@ *) }:         *@*                      #:              
                    =[: ]>:}+ +}= }- +* )> =) ] ++           -[-                   )=               
                   :]: -=-#-:[+  +] :[  *#  ) [:=[             >)                *@=                
                   }*    >  %=   [- [*  *#  ) <:-#              :%*            +#*]                 
                  :%       :=   }+ :}   ]-  + >:-#             :)]>%+       :)<: ]:                 
                  -}           >)  >=           =%       ->)-=[>    :<]))))<-   }*                  
                  -}           }*->)            +]:-*]%}<%[)[:                 ]=                   
                  -}          -) <<             ><-:  <%#>-                   [+                    
                  -}          %-<<             :[                           :[-                     
                  :%           >>             :}                           >[:                      
                   *)         :]              [-                         =}=                        
                    )+        )*            +]                         +]=                          
                   =}}-       ]=           >*                       :>[:                            
                 >}:                                              >%+                               
              -[<:                                            +])+:                                 
            +[+              :=+><<<<<)<<*+==-:::::--==+<<][)=                                      
          >}-           :)%}<-:            :::::::::::                                              
        -}=         -][>-                                                                           
       *]         *}=                                                                               
      +)        +@-                                                                                 
     =}       :[*                                                                                   
     ):      *]                                                                                     
    -<     :}*                                                                                      
    +*   *[<:                                                                    -:-:::-:-:-::--:   
     <]<=                                                                               :           
                                                                                                    
A production-grade, governed AI agent system with a chat-first UI interface.

## Architecture

- **Otter-AI**: Go-based backend implementing governed, stateful AI agent
- **Kelpie UI**: React + TypeScript chat interface

## Features

- **Governance System**: Raft-based consensus with executable rules
- **Memory Layer**: Vector database for bounded, auditable memory
- **LLM Abstraction**: Pluggable providers (Ollama, OpenAI, Anthropic, OpenWebUI)
- **Plugin System**: Discord, Signal, Telegram, Slack integrations
- **Cryptography**: Hybrid ECDH + Kyber key exchange with AES-256
- **Local-First**: Containerized, runs entirely locally

## Quick Start

### Prerequisites

- Docker
- Docker Compose
- Make

### Build and Run

```bash
# Start all services
make all up

# Start individual services
make otter up
make kelpie up

# Stop services
make all down

# Purge all data and containers
make all purge
```

### Access

- **Kelpie UI**: http://localhost:3000
- **Otter-AI API**: http://localhost:8080
- **Health Check**: http://localhost:8080/health

## Configuration

Copy `.env.example` to `.env` in the `otter-ai/` directory and configure:

```bash
cp otter-ai/.env.example otter-ai/.env
```

Required configuration:
- `OTTER_RAFT_ID`: Unique identifier for this Otter instance
- `OTTER_RAFT_TYPE`: `super-raft`, `raft`, or `sub-raft`
- `OTTER_LLM_PROVIDER`: LLM provider (ollama, openai, anthropic, openwebui)
- `OTTER_LLM_ENDPOINT`: LLM endpoint URL
- `OTTER_LLM_MODEL`: Model name

## API Endpoints

### Chat
- `POST /api/v1/chat` - Send a message

### Memory
- `GET /api/v1/memories` - List memories
- `POST /api/v1/memories` - Create a memory
- `DELETE /api/v1/memories/{id}` - Delete a memory

### Governance
- `GET /api/v1/governance/rules` - List active rules
- `POST /api/v1/governance/rules` - Propose a new rule
- `POST /api/v1/governance/vote` - Vote on a proposal
- `GET /api/v1/governance/members` - List raft members

## Development

### Otter-AI (Backend)

```bash
cd otter-ai
go mod download
go run cmd/otter/main.go
```

### Kelpie UI (Frontend)

```bash
cd kelpie-ui
npm install
npm run dev
```

## Governance Model

### Raft Types
- **Super-Raft (Raft 0)**: Induction authority, global overrides
- **Raft (Raft 1)**: Membership gatekeeper, quorum enforcement
- **Sub-Raft (Raft 2+)**: Regular members

### Membership States
- `active`: Can vote and propose
- `inactive`: Temporarily inactive
- `expired`: 90 days of inactivity
- `revoked`: Membership revoked
- `left`: Voluntarily left

### Voting
- **Simple Majority**: YES > (YES + NO) / 2
- **Super-Majority**: YES > 75% of (YES + NO) (for overrides)
- **Quorum**: ≥ 50% active members must vote

## Security

- Hybrid ECDH + Kyber key exchange
- AES-256-GCM encryption
- All governance messages signed
- Fail-closed on cryptographic failures
- Keys automatically generated on first run
- Private keys stored in `/data/otter.key` (600 permissions)
- Public keys distributed during raft membership induction

### Key Management

Keys are **automatically generated** when Otter-AI first starts:
- Private key: ECDH P-256, stored in `$OTTER_RAFT_DATA_DIR/otter.key`
- Public key: Derived from private key, stored in member record
- Keys persist across restarts
- Each Otter instance has a unique key pair

To view your public key:
```bash
# Keys are stored as hex in the data directory
cat /data/raft/otter.key
```

**Important**: Backup your private key! Losing it means losing your governance identity.

## License

Proprietary - See LICENSE file

## Support

For issues and questions, please open an issue on the repository.
