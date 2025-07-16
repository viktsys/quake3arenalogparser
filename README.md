# Quake Log Parser - AI-Driven Live Coding Test

> **Truth can only be found in one place: the code.**  
> – Robert C. Martin  

## Introduction

Welcome to the **Quake Log Parser Test**! In this live coding challenge, you will using embedded AI tools in your IDE (e.g., Aider, Cursor, Co-pilot) to implement a log parser that processes game data from a Quake 3 Arena server. This test evaluates your technical skills, problem-solving abilities, and effective use of AI in real-time coding.

---

## The Challenge

### Task: Parse a Quake log file to extract and summarize match data.

1. **Input**: A log file with match events, such as player kills and deaths.
Example Snippet:
```
0:00 Kill: 1022 2 22: <world> killed Isgalamido by MOD_TRIGGER_HURT
0:15 Kill: 3 2 10: Isgalamido killed Dono da Bola by MOD_RAILGUN
1:00 Kill: 3 2 10: Isgalamido killed Zeh by MOD_RAILGUN
```

[Full Log](https://gist.githubusercontent.com/cloudwalk-tests/be1b636e58abff14088c8b5309f575d8/raw/df6ef4a9c0b326ce3760233ef24ae8bfa8e33940/qgames.log)

2. **Output**: JSON summarizing players and their kills.
Example:
```json
{
  "players": ["Isgalamido", "Dono da Bola", "Zeh"],
  "kills": {"Isgalamido": 2, "Dono da Bola": 0, "Zeh": 0}
}
```

---

## Requirements

1. Parse the log file and extract the following:
   - A list of players in the match.
   - A dictionary showing the number of kills per player.

2. **Rules**:
   - If `<world>` kills a player, that player’s kill count decreases by 1.
   - `<world>` is not considered a player and should not appear in the output.


---

## Plus Features (Optional)

For candidates who finish the base task quickly, implement one or more of these advanced features:

1. **Group Data by Match**  
   Create a structure that organizes the parsed data by match.  
Example Output:
```json
{
  "game_1": {
    "total_kills": 3,
    "players": ["Isgalamido", "Dono da Bola", "Zeh"],
    "kills": {"Isgalamido": 2, "Dono da Bola": 0, "Zeh": 0}
  }
}
```   

2. **Death Cause Report**  
   Summarize kills grouped by the cause of death.  
Example:
```json
{
  "game_1": {
    "kills_by_means": {
      "MOD_TRIGGER_HURT": 1,
      "MOD_RAILGUN": 2
    }
  }
}
```

3. **Ranking Report**  
   Generate a ranking of players based on their kill counts across all matches.
Example:
```
1. Isgalamido - 10 kills
2. Zeh - 7 kills
3. Dono da Bola - 3 kills
```  