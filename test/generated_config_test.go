package test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"gameserver/common/config"
	genConfig "gameserver/common/config/generated"
)

// TestGeneratedConfigs 测试生成的配置文件
func TestGeneratedConfigs(t *testing.T) {
	// 创建临时测试目录
	testDir, err := ioutil.TempDir("", "generated_config_test")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(testDir)

	// 初始化全局配置管理器
	config.InitGlobalConfig(testDir)

	// 创建配置管理器（用于测试）
	cm := config.NewConfigManager(testDir)

	t.Run("ItemConfig", func(t *testing.T) {
		testItemConfig(t, cm, testDir)
	})

	t.Run("MonsterConfig", func(t *testing.T) {
		testMonsterConfig(t, cm, testDir)
	})

	t.Run("SkillConfig", func(t *testing.T) {
		testSkillConfig(t, cm, testDir)
	})

	t.Run("ConfigCaching", func(t *testing.T) {
		testConfigCaching(t, cm, testDir)
	})

	t.Run("ConfigReload", func(t *testing.T) {
		testConfigReload(t, cm, testDir)
	})
}

// testItemConfig 测试物品配置
func testItemConfig(t *testing.T, cm *config.ConfigManager, testDir string) {
	// 创建测试物品配置
	testItems := []map[string]interface{}{
		{"id": "1001", "name": "铁剑", "type": "weapon", "attack": 15, "durability": 100, "price": 1000},
		{"id": "1002", "name": "皮甲", "type": "armor", "defense": 8, "durability": 80, "price": 800},
		{"id": "1003", "name": "生命药水", "type": "potion", "heal": 50, "price": 100},
	}

	configData, err := json.Marshal(testItems)
	if err != nil {
		t.Fatalf("序列化物品配置失败: %v", err)
	}

	configFile := filepath.Join(testDir, "items.json")
	err = ioutil.WriteFile(configFile, configData, 0644)
	if err != nil {
		t.Fatalf("写入物品配置文件失败: %v", err)
	}

	// 先通过全局管理器加载配置
	err = config.LoadConfig("items.json")
	if err != nil {
		t.Fatalf("全局加载物品配置失败: %v", err)
	}

	// 测试获取单个物品配置
	item, exists := genConfig.GetItemConfig("1001")
	if !exists {
		t.Error("应该能找到ID为1001的物品配置")
	}

	if item == nil {
		t.Error("物品配置不应该为nil")
	}

	if item.Name != "铁剑" {
		t.Errorf("期望名称为'铁剑'，实际为'%s'", item.Name)
	}

	if item.Type != "weapon" {
		t.Errorf("期望类型为'weapon'，实际为'%s'", item.Type)
	}

	if item.Attack != 15 {
		t.Errorf("期望攻击力为15，实际为%v", item.Attack)
	}

	// 测试获取不存在的物品
	_, exists = genConfig.GetItemConfig("9999")
	if exists {
		t.Error("不应该能找到ID为9999的物品配置")
	}

	// 测试获取所有物品配置
	allItems, exists := genConfig.GetAllItemConfigs()
	if !exists {
		t.Error("应该能找到所有物品配置")
	}

	if len(allItems) != 3 {
		t.Errorf("期望有3个物品配置，实际有%d个", len(allItems))
	}

	// 测试获取物品名称
	name, exists := genConfig.GetItemName("1002")
	if !exists {
		t.Error("应该能找到ID为1002的物品名称")
	}

	if name != "皮甲" {
		t.Errorf("期望名称为'皮甲'，实际为'%s'", name)
	}

	// 测试验证配置
	err = genConfig.ValidateItemConfig("1003")
	if err != nil {
		t.Errorf("验证有效配置不应该失败: %v", err)
	}

	err = genConfig.ValidateItemConfig("9999")
	if err == nil {
		t.Error("验证无效配置应该失败")
	}
}

// testMonsterConfig 测试怪物配置
func testMonsterConfig(t *testing.T, cm *config.ConfigManager, testDir string) {
	// 创建测试怪物配置
	testMonsters := []map[string]interface{}{
		{"id": "2001", "name": "哥布林", "type": "normal", "hp": 100, "attack": 20, "defense": 5, "exp": 50},
		{"id": "2002", "name": "兽人", "type": "elite", "hp": 200, "attack": 35, "defense": 15, "exp": 100},
		{"id": "2003", "name": "龙", "type": "boss", "hp": 1000, "attack": 80, "defense": 40, "exp": 500},
	}

	configData, err := json.Marshal(testMonsters)
	if err != nil {
		t.Fatalf("序列化怪物配置失败: %v", err)
	}

	configFile := filepath.Join(testDir, "monsters.json")
	err = ioutil.WriteFile(configFile, configData, 0644)
	if err != nil {
		t.Fatalf("写入怪物配置文件失败: %v", err)
	}

	// 先通过全局管理器加载配置
	err = config.LoadConfig("monsters.json")
	if err != nil {
		t.Fatalf("全局加载怪物配置失败: %v", err)
	}

	// 测试获取单个怪物配置
	monster, exists := genConfig.GetMonsterConfig("2001")
	if !exists {
		t.Error("应该能找到ID为2001的怪物配置")
	}

	if monster == nil {
		t.Error("怪物配置不应该为nil")
	}

	if monster.Name != "哥布林" {
		t.Errorf("期望名称为'哥布林'，实际为'%s'", monster.Name)
	}

	if monster.Type != "normal" {
		t.Errorf("期望类型为'normal'，实际为'%s'", monster.Type)
	}

	if monster.Hp != 100 {
		t.Errorf("期望HP为100，实际为%v", monster.Hp)
	}

	// 测试获取所有怪物配置
	allMonsters, exists := genConfig.GetAllMonsterConfigs()
	if !exists {
		t.Error("应该能找到所有怪物配置")
	}

	if len(allMonsters) != 3 {
		t.Errorf("期望有3个怪物配置，实际有%d个", len(allMonsters))
	}

	// 测试获取怪物名称
	name, exists := genConfig.GetMonsterName("2002")
	if !exists {
		t.Error("应该能找到ID为2002的怪物名称")
	}

	if name != "兽人" {
		t.Errorf("期望名称为'兽人'，实际为'%s'", name)
	}

	// 测试验证配置
	err = genConfig.ValidateMonsterConfig("2003")
	if err != nil {
		t.Errorf("验证有效配置不应该失败: %v", err)
	}
}

// testSkillConfig 测试技能配置
func testSkillConfig(t *testing.T, cm *config.ConfigManager, testDir string) {
	// 创建测试技能配置
	testSkills := []map[string]interface{}{
		{"id": "3001", "name": "火球术", "type": "attack", "damage": 50, "mana_cost": 30, "cooldown": 3.0, "range": 10.5},
		{"id": "3002", "name": "治疗术", "type": "heal", "heal": 80, "mana_cost": 40, "cooldown": 5.0, "targets": []string{"ally", "self"}},
		{"id": "3003", "name": "护盾术", "type": "buff", "defense": 20, "mana_cost": 25, "duration": 10.0, "effects": []string{"defense_up"}},
	}

	configData, err := json.Marshal(testSkills)
	if err != nil {
		t.Fatalf("序列化技能配置失败: %v", err)
	}

	configFile := filepath.Join(testDir, "skills.json")
	err = ioutil.WriteFile(configFile, configData, 0644)
	if err != nil {
		t.Fatalf("写入技能配置文件失败: %v", err)
	}

	// 先通过全局管理器加载配置
	err = config.LoadConfig("skills.json")
	if err != nil {
		t.Fatalf("全局加载技能配置失败: %v", err)
	}

	// 测试获取单个技能配置
	skill, exists := genConfig.GetSkillConfig("3001")
	if !exists {
		t.Error("应该能找到ID为3001的技能配置")
	}

	if skill == nil {
		t.Error("技能配置不应该为nil")
	}

	if skill.Name != "火球术" {
		t.Errorf("期望名称为'火球术'，实际为'%s'", skill.Name)
	}

	if skill.Type != "attack" {
		t.Errorf("期望类型为'attack'，实际为'%s'", skill.Type)
	}

	if skill.Damage != 50 {
		t.Errorf("期望伤害为50，实际为%v", skill.Damage)
	}

	if skill.Mana != 30 {
		t.Errorf("期望魔法消耗为30，实际为%v", skill.Mana)
	}

	if skill.Cooldown != 3.0 {
		t.Errorf("期望冷却时间为3.0，实际为%v", skill.Cooldown)
	}

	if skill.Range != 10.5 {
		t.Errorf("期望射程为10.5，实际为%v", skill.Range)
	}

	// 测试获取所有技能配置
	allSkills, exists := genConfig.GetAllSkillConfigs()
	if !exists {
		t.Error("应该能找到所有技能配置")
	}

	if len(allSkills) != 3 {
		t.Errorf("期望有3个技能配置，实际有%d个", len(allSkills))
	}

	// 测试获取技能名称
	name, exists := genConfig.GetSkillName("3002")
	if !exists {
		t.Error("应该能找到ID为3002的技能名称")
	}

	if name != "治疗术" {
		t.Errorf("期望名称为'治疗术'，实际为'%s'", name)
	}

	// 测试验证配置
	err = genConfig.ValidateSkillConfig("3003")
	if err != nil {
		t.Errorf("验证有效配置不应该失败: %v", err)
	}
}

// testConfigCaching 测试配置缓存功能
func testConfigCaching(t *testing.T, cm *config.ConfigManager, testDir string) {
	// 创建测试配置
	testItems := []map[string]interface{}{
		{"id": "1001", "name": "测试物品", "type": "weapon", "attack": 10},
	}

	configData, err := json.Marshal(testItems)
	if err != nil {
		t.Fatalf("序列化测试配置失败: %v", err)
	}

	configFile := filepath.Join(testDir, "cache_test.json")
	err = ioutil.WriteFile(configFile, configData, 0644)
	if err != nil {
		t.Fatalf("写入测试配置文件失败: %v", err)
	}

	// 先通过全局管理器加载配置
	err = config.LoadConfig("cache_test.json")
	if err != nil {
		t.Fatalf("全局加载测试配置失败: %v", err)
	}

	// 由于生成的配置代码只处理特定的文件名，我们需要通过原始接口测试缓存
	// 测试原始配置获取
	config1, exists := config.GetConfig("cache_test.json", "1001")
	if !exists {
		t.Error("应该能找到ID为1001的测试配置")
	}

	if config1 == nil {
		t.Error("测试配置不应该为nil")
	}

	// 验证配置内容
	if configMap, ok := config1.(map[string]interface{}); ok {
		if name, ok := configMap["name"].(string); !ok || name != "测试物品" {
			t.Errorf("期望名称为'测试物品'，实际为'%v'", name)
		}
	} else {
		t.Error("配置应该可以转换为map[string]interface{}")
	}

	// 测试第二次获取（这里没有缓存，但可以验证一致性）
	config2, exists := config.GetConfig("cache_test.json", "1001")
	if !exists {
		t.Error("应该能再次获取测试配置")
	}

	if config2 == nil {
		t.Error("再次获取的测试配置不应该为nil")
	}

	// 测试获取所有配置
	allConfigs, exists := config.GetAllConfigs("cache_test.json")
	if !exists {
		t.Error("应该能找到所有测试配置")
	}

	if len(allConfigs) != 1 {
		t.Errorf("期望有1个测试配置，实际有%d个", len(allConfigs))
	}
}

// testConfigReload 测试配置重载功能
func testConfigReload(t *testing.T, cm *config.ConfigManager, testDir string) {
	// 创建初始配置
	initialItems := []map[string]interface{}{
		{"id": "1001", "name": "初始物品", "type": "weapon", "attack": 10},
	}

	configData, err := json.Marshal(initialItems)
	if err != nil {
		t.Fatalf("序列化初始配置失败: %v", err)
	}

	configFile := filepath.Join(testDir, "reload_test.json")
	err = ioutil.WriteFile(configFile, configData, 0644)
	if err != nil {
		t.Fatalf("写入初始配置文件失败: %v", err)
	}

	// 先通过全局管理器加载初始配置
	err = config.LoadConfig("reload_test.json")
	if err != nil {
		t.Fatalf("全局加载初始配置失败: %v", err)
	}

	// 获取初始配置
	config1, exists := config.GetConfig("reload_test.json", "1001")
	if !exists {
		t.Error("应该能找到初始物品配置")
	}

	if config1 == nil {
		t.Error("初始物品配置不应该为nil")
	}

	// 验证初始配置
	if configMap, ok := config1.(map[string]interface{}); ok {
		if name, ok := configMap["name"].(string); !ok || name != "初始物品" {
			t.Errorf("期望名称为'初始物品'，实际为'%v'", name)
		}
		if attack, ok := configMap["attack"].(float64); !ok || attack != 10 {
			t.Errorf("期望攻击力为10，实际为%v", attack)
		}
	} else {
		t.Error("配置应该可以转换为map[string]interface{}")
	}

	// 修改配置文件
	updatedItems := []map[string]interface{}{
		{"id": "1001", "name": "更新后的物品", "type": "weapon", "attack": 15},
	}

	updatedData, err := json.Marshal(updatedItems)
	if err != nil {
		t.Fatalf("序列化更新配置失败: %v", err)
	}

	err = ioutil.WriteFile(configFile, updatedData, 0644)
	if err != nil {
		t.Fatalf("写入更新配置文件失败: %v", err)
	}

	// 重新加载配置
	err = config.ReloadConfig("reload_test.json")
	if err != nil {
		t.Errorf("重新加载配置失败: %v", err)
	}

	// 验证配置已更新
	updatedConfig, exists := config.GetConfig("reload_test.json", "1001")
	if !exists {
		t.Error("重新加载后应该能找到物品配置")
	}

	if updatedConfig == nil {
		t.Error("重新加载后的配置不应该为nil")
	}

	// 验证更新后的配置
	if configMap, ok := updatedConfig.(map[string]interface{}); ok {
		if name, ok := configMap["name"].(string); !ok || name != "更新后的物品" {
			t.Errorf("期望名称为'更新后的物品'，实际为'%v'", name)
		}
		if attack, ok := configMap["attack"].(float64); !ok || attack != 15 {
			t.Errorf("期望攻击力为15，实际为%v", attack)
		}
	} else {
		t.Error("重新加载后的配置应该可以转换为map[string]interface{}")
	}
}

// TestGeneratedConfigEdgeCases 测试生成的配置边界情况
func TestGeneratedConfigEdgeCases(t *testing.T) {
	testDir, err := ioutil.TempDir("", "generated_config_edge_test")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(testDir)

	cm := config.NewConfigManager(testDir)

	t.Run("EmptyConfig", func(t *testing.T) {
		// 测试空配置
		emptyConfig := []map[string]interface{}{}
		configData, _ := json.Marshal(emptyConfig)
		configFile := filepath.Join(testDir, "empty.json")
		ioutil.WriteFile(configFile, configData, 0644)

		err := cm.LoadConfig("empty.json")
		if err != nil {
			t.Errorf("加载空配置不应该失败: %v", err)
		}

		// 尝试获取空配置
		_, exists := genConfig.GetItemConfig("1001")
		if exists {
			t.Error("空配置中不应该能找到任何物品")
		}
	})

	t.Run("InvalidConfigData", func(t *testing.T) {
		// 测试无效的配置数据
		invalidConfig := []map[string]interface{}{
			{"name": "无ID物品", "type": "weapon"}, // 缺少ID字段
		}
		configData, _ := json.Marshal(invalidConfig)
		configFile := filepath.Join(testDir, "invalid.json")
		ioutil.WriteFile(configFile, configData, 0644)

		err := cm.LoadConfig("invalid.json")
		if err != nil {
			t.Errorf("加载包含无效数据的配置不应该失败: %v", err)
		}

		// 应该无法获取无效配置
		_, exists := genConfig.GetItemConfig("1001")
		if exists {
			t.Error("无效配置中不应该能找到物品")
		}
	})
}

// BenchmarkGeneratedConfig 生成的配置性能基准测试
func BenchmarkGeneratedConfig(b *testing.B) {
	testDir, err := ioutil.TempDir("", "generated_config_benchmark")
	if err != nil {
		b.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(testDir)

	cm := config.NewConfigManager(testDir)

	// 创建大量测试配置
	largeConfig := make([]map[string]interface{}, 1000)
	for i := 0; i < 1000; i++ {
		largeConfig[i] = map[string]interface{}{
			"id":     fmt.Sprintf("%d", i),
			"name":   fmt.Sprintf("物品%d", i),
			"type":   "weapon",
			"attack": i,
		}
	}

	configData, _ := json.Marshal(largeConfig)
	configFile := filepath.Join(testDir, "large.json")
	ioutil.WriteFile(configFile, configData, 0644)

	// 加载配置
	err = cm.LoadConfig("large.json")
	if err != nil {
		b.Fatalf("加载配置失败: %v", err)
	}

	b.ResetTimer()

	// 基准测试：获取物品配置（带缓存）
	b.Run("GetItemConfig", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, exists := genConfig.GetItemConfig("500")
			if !exists {
				b.Fatal("配置丢失")
			}
		}
	})

	// 基准测试：获取所有物品配置
	b.Run("GetAllItemConfigs", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, exists := genConfig.GetAllItemConfigs()
			if !exists {
				b.Fatal("配置丢失")
			}
		}
	})

	// 基准测试：获取物品名称
	b.Run("GetItemName", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, exists := genConfig.GetItemName("500")
			if !exists {
				b.Fatal("配置丢失")
			}
		}
	})
}
