# frozen_string_literal: true

require "spec_helper"

RSpec.describe Emb do
  before(:all) { Emb.setup(port: 16379) }
  after(:all) { Emb.reset_registry! }

  describe ".[]" do
    it "returns a proxy" do
      expect(Emb[:test]).to be_a(Emb::Proxy)
    end

    it "memoizes the proxy" do
      expect(Emb[:test]).to equal(Emb[:test])
    end
  end

  describe ".ping" do
    it "returns PONG" do
      expect(Emb.ping).to eq("PONG")
    end
  end

  describe ".models" do
    it "returns an array of model hashes" do
      models = Emb.models
      expect(models).to be_an(Array)
      expect(models.first).to have_key(:name)
      expect(models.first).to have_key(:dim)
      expect(models.first).to have_key(:status)
    end
  end

  describe ".info" do
    it "returns model info as a hash" do
      info = Emb.info(:minilm)
      expect(info).to be_a(Hash)
      expect(info[:dim]).to eq(384)
    end
  end

  describe ".help" do
    it "returns help text" do
      help = Emb.help
      expect(help).to be_a(String)
      expect(help).to include("EMB")
    end
  end

  describe "Proxy" do
    it "embeds text and returns an array of floats" do
      result = Emb[:minilm]["hello world"]
      expect(result).to be_a(Array)
      expect(result.size).to eq(384)
      expect(result.first).to be_a(Float)
    end

    it "embeds multiple texts" do
      results = Emb[:minilm]["hello", "world"]
      expect(results).to be_an(Array)
      expect(results.length).to eq(2)
      expect(results.first).to be_a(Array)
      expect(results.first.size).to eq(384)
    end
  end

  describe ".multi" do
    it "returns unpacked float arrays" do
      results = Emb.multi do |m|
        m[:minilm]["hello"]
        m[:minilm]["world"]
      end
      expect(results).to be_an(Array)
      expect(results.length).to eq(2)
      expect(results.first).to be_a(Array)
      expect(results.first.size).to eq(384)
      expect(results.first.first).to be_a(Float)
    end
  end

  describe ".new" do
    it "returns an Emb::Client instance" do
      client = Emb.new(port: 16379)
      expect(client).to be_a(Emb::Client)
      expect(client.ping).to eq("PONG")
    end

    it "supports url parameter" do
      client = Emb.new(url: "redis://localhost:16379")
      expect(client).to be_a(Emb::Client)
      expect(client.ping).to eq("PONG")
    end

    it "supports pool size parameter" do
      client = Emb.new(port: 16379, pool: 3)
      expect(client.pool.size).to eq(3)
    end

    it "defaults to EMB_URL env var" do
      allow(ENV).to receive(:[]).and_call_original
      allow(ENV).to receive(:[]).with("EMB_URL").and_return("redis://localhost:16379")
      client = Emb.new
      expect(client.ping).to eq("PONG")
    end

    it "creates independent clients" do
      c1 = Emb.new(port: 16379)
      c2 = Emb.new(port: 16379)
      expect(c1).not_to equal(c2)
      expect(c1[:minilm]).not_to equal(c2[:minilm])
    end

    it "supports all command methods" do
      client = Emb.new(port: 16379)
      expect(client.models).to be_an(Array)
      expect(client.info(:minilm)).to be_a(Hash)
      expect(client.help).to be_a(String)
      expect(client.ping).to eq("PONG")
    end

    it "supports multi on instance" do
      client = Emb.new(port: 16379)
      results = client.multi do |m|
        m[:minilm]["hello"]
        m[:minilm]["world"]
      end
      expect(results).to be_an(Array)
      expect(results.length).to eq(2)
      expect(results.first).to be_a(Array)
      expect(results.first.size).to eq(384)
      expect(results.first.first).to be_a(Float)
    end
  end
end
