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
    +*   *[<:                                                                       
     <]<=                                                                                          
                                                                                                    
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

**Authentication**: If `OTTER_HOST_PASSPHRASE` is configured, you'll need to authenticate when accessing Kelpie UI. The API will require a Bearer token obtained from the `/auth` endpoint.

## Configuration

Copy `.env.example` to `.env` in the `otter-ai/` directory and configure:

```bash
cp otter-ai/.env.example otter-ai/.env
```

Required configuration:
- `OTTER_RAFT_ID`: Unique identifier for this Otter instance
- `OTTER_LLM_PROVIDER`: LLM provider (ollama, openai, anthropic, openwebui)
- `OTTER_LLM_ENDPOINT`: LLM endpoint URL
- `OTTER_LLM_MODEL`: Model name

Optional security configuration:
- `OTTER_HOST_PASSPHRASE`: Passphrase to protect API and Kelpie UI access. Leave empty or unset to disable authentication.

## API Endpoints

### Authentication
- `POST /auth` - Authenticate with passphrase (if `OTTER_HOST_PASSPHRASE` is configured)
  - Request: `{"passphrase": "your-passphrase"}`
  - Response: `{"token": "jwt-token"}`
  - Use the token in subsequent requests: `Authorization: Bearer <token>`

**Note**: All endpoints below require authentication if `OTTER_HOST_PASSPHRASE` is set.

### Chat
- `POST /api/v1/chat` - Send a message

### Memory
- `GET /api/v1/memories` - List memories (read-only)

**Note**: Memories and musings can only be created and modified by the Otter agent internally. No public API endpoints are provided for creating or deleting memories to ensure the agent maintains full control over its own memory and reflection processes.

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

### Multi-Raft Membership
- **Individual Rafts**: Every otter starts as its own raft of 1 member
- **Multiple Memberships**: Otters can join N number of rafts simultaneously
- **Rule Adoption**: When joining a raft, the otter adopts all of that raft's rules
- **Peer Rafts**: Rafts become peers when their members overlap and rules are compatible

### Raft Joining Process
1. **Request Join**: Otter A requests to join Otter B's raft
2. **Rule Check**: System checks for conflicts between Otter A's existing raft rules and Otter B's raft rules
3. **No Conflicts**: If no conflicts, Otter A adopts Otter B's raft rules and joins immediately
4. **Conflicts Found**: If conflicts exist, LLM negotiation begins:
   - Both rafts' LLM backends discuss and negotiate a common rule amendment
   - Negotiated amendment is proposed to both rafts separately
   - Each raft votes on the amendment using their normal voting rules
5. **Both Adopt**: If both rafts adopt the amendment, rafts become peers and sharing begins
6. **Either Rejects**: If either raft rejects, the join request is dissolved

### Rule Conflicts
- Rules conflict when they have the same scope but different implementations
- Example: Both rafts have a "data_retention" rule with different time periods
- Conflicts trigger automatic LLM-based negotiation

### Membership States
- `active`: Can vote and propose
- `inactive`: Temporarily inactive
- `expired`: 90 days of inactivity
- `revoked`: Membership revoked
- `left`: Voluntarily left

### Voting
- **Solo Otter (1 member)**: Auto-adopts any rule immediately
- **Two Otters (2 members)**: Unanimous consent required (both must vote YES)
- **Three+ Otters (3+ members)**: 2/3 majority of total active members required
- **Super-Majority**: 75% of total active members (for rule overrides)
- **Quorum**: 2/3 of active members must participate (3+ member rafts)

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

Proprietary - See LICENSE file (AGPL v3.0)

## Support

For issues and questions, please open an issue on the repository.
