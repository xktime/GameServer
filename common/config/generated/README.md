# 生成的配置代码使用说明

这个目录包含了由配置生成器自动生成的Go代码文件，每个JSON配置文件对应一个Go文件。

## 生成的文件

- `item.go` - 物品配置 (`items.json`)
- `monster.go` - 怪物配置 (`monsters.json`)  
- `skill.go` - 技能配置 (`skills.json`)

## 使用方法

### 1. 导入生成的包

```go
import "your_project/common/config/generated"
```

### 2. 使用配置结构体

每个配置文件都会生成对应的结构体，例如：

```go
// 物品配置结构体
type Item struct {
    ID          string `json:"id"`          // 配置ID
    Name        string `json:"name"`        // 名称
    Type        string `json:"type"`        // 类型
    Attack      int    `json:"attack"`      // 攻击力
    Durability  int    `json:"durability"`  // 耐久度
    Price       int    `json:"price"`       // 价格
    Description string `json:"description"` // 描述
}

// 怪物配置结构体
type Monster struct {
    ID      string   `json:"id"`      // 配置ID
    Name    string   `json:"name"`    // 名称
    Type    string   `json:"type"`    // 类型
    Level   int      `json:"level"`   // 等级
    Hp      int      `json:"hp"`      // 血量
    Attack  int      `json:"attack"`  // 攻击力
    Defense int      `json:"defense"` // 防御力
    Exp     int      `json:"exp"`     // 经验值
    Drops   []string `json:"drops"`   // 掉落物品
}

// 技能配置结构体
type Skill struct {
    ID          string   `json:"id"`           // 配置ID
    Name        string   `json:"name"`         // 名称
    Mana        int      `json:"mana_cost"`    // 魔法消耗
    Range       int      `json:"range"`        // 施法范围
    Description string   `json:"description"`  // 描述
    Unlock      int      `json:"unlock_level"` // 解锁等级
    Type        string   `json:"type"`         // 类型
    Level       int      `json:"level"`        // 等级
    Damage      int      `json:"damage"`       // 伤害
    Cooldown    int      `json:"cooldown"`     // 冷却时间
    Effects     []string `json:"effects"`      // 效果
}
```

### 3. 使用配置访问函数

每个配置类型都提供以下函数：

#### 获取单个配置
```go
// 根据ID获取配置
item, exists := GetItemConfig("1001")
if exists {
    fmt.Printf("物品名称: %s, 攻击力: %d\n", item.Name, item.Attack)
}

monster, exists := GetMonsterConfig("1001")
if exists {
    fmt.Printf("怪物名称: %s, 等级: %d\n", monster.Name, monster.Level)
}

skill, exists := GetSkillConfig("1001")
if exists {
    fmt.Printf("技能名称: %s, 伤害: %d\n", skill.Name, skill.Damage)
}
```

#### 获取所有配置
```go
// 获取所有物品配置
allItems, exists := GetAllItemConfigs()
if exists {
    for id, item := range allItems {
        fmt.Printf("物品 %s: %s\n", id, item.Name)
    }
}

// 获取所有怪物配置
allMonsters, exists := GetAllMonsterConfigs()
if exists {
    for id, monster := range allMonsters {
        fmt.Printf("怪物 %s: %s (等级%d)\n", id, monster.Name, monster.Level)
    }
}

// 获取所有技能配置
allSkills, exists := GetAllSkillConfigs()
if exists {
    for id, skill := range allSkills {
        fmt.Printf("技能 %s: %s (类型:%s)\n", id, skill.Name, skill.Type)
    }
}
```

#### 获取配置名称
```go
// 获取物品名称
name, exists := GetItemName("1001")
if exists {
    fmt.Printf("物品1001名称: %s\n", name)
}

// 获取怪物名称
name, exists := GetMonsterName("1001")
if exists {
    fmt.Printf("怪物1001名称: %s\n", name)
}

// 获取技能名称
name, exists := GetSkillName("1001")
if exists {
    fmt.Printf("技能1001名称: %s\n", name)
}
```

#### 重载配置
```go
// 重载特定配置
err := ReloadItemConfig()
if err != nil {
    log.Printf("重载物品配置失败: %v", err)
}

err = ReloadMonsterConfig()
if err != nil {
    log.Printf("重载怪物配置失败: %v", err)
}

err = ReloadSkillConfig()
if err != nil {
    log.Printf("重载技能配置失败: %v", err)
}
```

#### 验证配置
```go
// 验证配置是否存在
err := ValidateItemConfig("1001")
if err != nil {
    log.Printf("物品1001配置验证失败: %v", err)
}

err = ValidateMonsterConfig("1001")
if err != nil {
    log.Printf("怪物1001配置验证失败: %v", err)
}

err = ValidateSkillConfig("1001")
if err != nil {
    log.Printf("技能1001配置验证失败: %v", err)
}
```

## 实际使用示例

### 游戏逻辑中使用配置

```go
func calculateDamage(weaponID, monsterID string) int {
    // 获取武器配置
    weapon, exists := GetItemConfig(weaponID)
    if !exists {
        return 0
    }
    
    // 获取怪物配置
    monster, exists := GetMonsterConfig(monsterID)
    if !exists {
        return 0
    }
    
    // 计算伤害
    damage := weapon.Attack - monster.Defense
    if damage < 0 {
        damage = 0
    }
    
    return damage
}

func canUseSkill(playerLevel int, skillID string) bool {
    skill, exists := GetSkillConfig(skillID)
    if !exists {
        return false
    }
    
    return playerLevel >= skill.Unlock
}
```

### 配置热重载

```go
func reloadGameConfigs() {
    // 重载所有配置
    if err := ReloadAll(); err != nil {
        log.Printf("重载配置失败: %v", err)
        return
    }
    
    log.Println("游戏配置重载成功")
}
```

## 注意事项

1. **类型安全**: 生成的代码使用反射进行类型转换，确保类型安全
2. **性能考虑**: 反射操作有一定性能开销，适合配置读取场景
3. **错误处理**: 所有函数都返回存在性标志，使用前请检查
4. **配置更新**: 当JSON配置文件更新后，需要重新运行生成器
5. **字段顺序**: ID字段始终在最前面，其他字段按JSON中的顺序排列

## 扩展配置

如果需要添加新的配置类型：

1. 在`conf/config/`目录下添加新的JSON文件
2. 运行配置生成器：`go run tools/config_generator/main.go`
3. 生成的代码会自动包含在项目中
4. 按照上述模式使用新生成的配置代码

## 故障排除

如果遇到问题：

1. 检查JSON文件格式是否正确
2. 确保JSON文件在`conf/config/`目录下
3. 重新运行配置生成器
4. 检查生成的Go代码是否有语法错误
5. 确保导入了正确的包路径
